package study

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/ashark-ai-05/tradefox/internal/config"
	"github.com/ashark-ai-05/tradefox/internal/core/enums"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

func testLogger() *slog.Logger {
	return slog.Default()
}

func newTestBus() *eventbus.Bus {
	return eventbus.NewBus(testLogger())
}

func newTestStudy() *BaseStudy {
	bus := newTestBus()
	settings := config.NewManager("")
	return NewBaseStudy("TestStudy", "1.0", "A test study", "tester", bus, settings, testLogger())
}

// TestBaseStudy_AddCalculation_NoAggregation verifies that with
// AggregationLevel=None, every item passes through to output immediately.
func TestBaseStudy_AddCalculation_NoAggregation(t *testing.T) {
	bs := newTestStudy()
	bs.SetAggregationLevel(enums.AggregationNone)

	ctx := context.Background()
	if err := bs.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer bs.StopAsync(ctx)

	outCh := bs.OnCalculated()

	// Send 5 items; each should pass through immediately.
	for i := 0; i < 5; i++ {
		bs.AddCalculation(models.BaseStudyModel{
			Value:     decimal.NewFromInt(int64(i + 1)),
			Timestamp: time.Now(),
		})
	}

	for i := 0; i < 5; i++ {
		select {
		case m := <-outCh:
			expected := decimal.NewFromInt(int64(i + 1))
			if !m.Value.Equal(expected) {
				t.Fatalf("item %d: expected value %s, got %s", i, expected, m.Value)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for item %d", i)
		}
	}
}

// TestBaseStudy_Aggregation_TimeBucketing verifies that items within the same
// time bucket are merged and a new bucket boundary fires output.
func TestBaseStudy_Aggregation_TimeBucketing(t *testing.T) {
	bs := newTestStudy()
	bs.SetAggregationLevel(enums.AggregationS1) // 1-second buckets

	// Custom aggregation: sum values.
	bs.OnDataAggregation = func(existing *models.BaseStudyModel, newItem models.BaseStudyModel, count int) {
		existing.Value = existing.Value.Add(newItem.Value)
	}

	ctx := context.Background()
	if err := bs.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer bs.StopAsync(ctx)

	outCh := bs.OnCalculated()

	// Create two items in the same 1-second bucket.
	now := time.Now().Truncate(time.Second)
	bs.AddCalculation(models.BaseStudyModel{
		Value:     decimal.NewFromInt(10),
		Timestamp: now,
	})
	bs.AddCalculation(models.BaseStudyModel{
		Value:     decimal.NewFromInt(20),
		Timestamp: now.Add(100 * time.Millisecond),
	})

	// Send an item in the NEXT bucket to flush the previous one.
	bs.AddCalculation(models.BaseStudyModel{
		Value:     decimal.NewFromInt(5),
		Timestamp: now.Add(1 * time.Second),
	})

	// The first output should be the aggregated bucket (10 + 20 = 30).
	select {
	case m := <-outCh:
		expected := decimal.NewFromInt(30)
		if !m.Value.Equal(expected) {
			t.Fatalf("expected aggregated value %s, got %s", expected, m.Value)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for aggregated output")
	}
}

// TestBaseStudy_Aggregation_SkipFlag verifies that items with
// AddItemSkippingAggregation=true always pass through immediately.
func TestBaseStudy_Aggregation_SkipFlag(t *testing.T) {
	bs := newTestStudy()
	bs.SetAggregationLevel(enums.AggregationS1) // 1-second buckets

	ctx := context.Background()
	if err := bs.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer bs.StopAsync(ctx)

	outCh := bs.OnCalculated()

	now := time.Now().Truncate(time.Second)

	// Send a normal item (will be bucketed).
	bs.AddCalculation(models.BaseStudyModel{
		Value:     decimal.NewFromInt(10),
		Timestamp: now,
	})

	// Send an item with skip flag - should pass through immediately.
	bs.AddCalculation(models.BaseStudyModel{
		Value:                      decimal.NewFromInt(99),
		Timestamp:                  now.Add(50 * time.Millisecond),
		AddItemSkippingAggregation: true,
	})

	// The skip-flag item should arrive first (the bucketed one is still waiting).
	select {
	case m := <-outCh:
		expected := decimal.NewFromInt(99)
		if !m.Value.Equal(expected) {
			t.Fatalf("expected skip-flag value %s, got %s", expected, m.Value)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for skip-flag item")
	}
}

// TestBaseStudy_StaleDetection verifies that when no data arrives for the
// configured stale duration, a stale model is emitted.
func TestBaseStudy_StaleDetection(t *testing.T) {
	bs := newTestStudy()
	bs.SetAggregationLevel(enums.AggregationNone)
	bs.SetStaleDuration(200 * time.Millisecond) // Short duration for testing.

	ctx := context.Background()
	if err := bs.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer bs.StopAsync(ctx)

	outCh := bs.OnCalculated()

	// Wait for stale indicator.
	select {
	case m := <-outCh:
		if !m.IsStale {
			t.Fatal("expected IsStale=true")
		}
		if m.ValueColor != "Orange" {
			t.Fatalf("expected ValueColor='Orange', got %q", m.ValueColor)
		}
		if m.Tooltip == "" {
			t.Fatal("expected non-empty Tooltip on stale model")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stale indicator")
	}
}

// TestBaseStudy_Lifecycle verifies that start/stop sets correct status and
// channels function properly.
func TestBaseStudy_Lifecycle(t *testing.T) {
	bs := newTestStudy()
	bs.SetAggregationLevel(enums.AggregationNone)

	// Initial status should be Loaded.
	if s := bs.Status(); s != enums.PluginLoaded {
		t.Fatalf("expected initial status PluginLoaded, got %s", s)
	}

	ctx := context.Background()
	if err := bs.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}

	if s := bs.Status(); s != enums.PluginStarted {
		t.Fatalf("expected status PluginStarted after start, got %s", s)
	}

	// Verify output channel works.
	bs.AddCalculation(models.BaseStudyModel{
		Value:     decimal.NewFromInt(42),
		Timestamp: time.Now(),
	})
	select {
	case m := <-bs.OnCalculated():
		if !m.Value.Equal(decimal.NewFromInt(42)) {
			t.Fatalf("expected value 42, got %s", m.Value)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for output after start")
	}

	if err := bs.StopAsync(ctx); err != nil {
		t.Fatalf("StopAsync failed: %v", err)
	}

	if s := bs.Status(); s != enums.PluginStopped {
		t.Fatalf("expected status PluginStopped after stop, got %s", s)
	}
}

// TestBaseStudy_Restart_Success verifies that HandleRestart can recover.
func TestBaseStudy_Restart_Success(t *testing.T) {
	bs := newTestStudy()
	bs.SetAggregationLevel(enums.AggregationNone)
	bs.SetStaleDuration(10 * time.Minute) // Disable stale detection for this test.

	ctx := context.Background()
	if err := bs.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}

	// Trigger a restart.
	bs.HandleRestart(ctx, "test failure", nil)

	// After successful restart, status should be Started.
	if s := bs.Status(); s != enums.PluginStarted {
		t.Fatalf("expected PluginStarted after restart, got %s", s)
	}

	// Verify the study still works after restart.
	bs.AddCalculation(models.BaseStudyModel{
		Value:     decimal.NewFromInt(77),
		Timestamp: time.Now(),
	})
	select {
	case m := <-bs.OnCalculated():
		if !m.Value.Equal(decimal.NewFromInt(77)) {
			t.Fatalf("expected value 77 after restart, got %s", m.Value)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for output after restart")
	}

	bs.StopAsync(ctx)
}

// TestBaseStudy_Restart_MaxAttempts verifies that after exhausting restart
// attempts the study status is set to StoppedFailed.
func TestBaseStudy_Restart_MaxAttempts(t *testing.T) {
	bs := newTestStudy()
	bs.SetAggregationLevel(enums.AggregationNone)
	bs.SetStaleDuration(10 * time.Minute)
	bs.maxRestartAttempts = 1 // Lower max for faster testing.

	ctx := context.Background()
	if err := bs.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer bs.StopAsync(ctx)

	// Simulate exhaustion: set attempt counter past max.
	bs.restartMu.Lock()
	bs.restartAttempt = 1
	bs.restartMu.Unlock()

	bs.HandleRestart(ctx, "final failure", nil)

	if s := bs.Status(); s != enums.PluginStoppedFailed {
		t.Fatalf("expected PluginStoppedFailed after max attempts, got %s", s)
	}
}

// TestBaseStudy_PluginUniqueID verifies the SHA256-based unique ID is
// consistent and deterministic.
func TestBaseStudy_PluginUniqueID(t *testing.T) {
	bs := newTestStudy()

	// Compute expected SHA256.
	h := sha256.New()
	h.Write([]byte("TestStudy"))
	h.Write([]byte("tester"))
	h.Write([]byte("1.0"))
	h.Write([]byte("A test study"))
	expected := hex.EncodeToString(h.Sum(nil))

	if bs.PluginUniqueID() != expected {
		t.Fatalf("expected PluginUniqueID %q, got %q", expected, bs.PluginUniqueID())
	}

	// Creating another study with the same metadata should produce the same ID.
	bs2 := NewBaseStudy("TestStudy", "1.0", "A test study", "tester", nil, nil, testLogger())
	if bs.PluginUniqueID() != bs2.PluginUniqueID() {
		t.Fatal("expected identical PluginUniqueIDs for same metadata")
	}

	// Different metadata should produce a different ID.
	bs3 := NewBaseStudy("OtherStudy", "1.0", "A test study", "tester", nil, nil, testLogger())
	if bs.PluginUniqueID() == bs3.PluginUniqueID() {
		t.Fatal("expected different PluginUniqueIDs for different metadata")
	}
}

// TestBaseStudy_OutputChannel verifies OnCalculated returns a receive-only channel.
func TestBaseStudy_OutputChannel(t *testing.T) {
	bs := newTestStudy()

	outCh := bs.OnCalculated()
	if outCh == nil {
		t.Fatal("OnCalculated() returned nil channel")
	}

	alertCh := bs.OnAlertTriggered()
	if alertCh == nil {
		t.Fatal("OnAlertTriggered() returned nil channel")
	}

	// Verify the channels are receive-only by checking we can read from them
	// (they should be non-nil buffered channels).
	// Write to the internal channels and read from the external ones.
	bs.outputCh <- models.BaseStudyModel{Value: decimal.NewFromInt(1)}
	select {
	case m := <-outCh:
		if !m.Value.Equal(decimal.NewFromInt(1)) {
			t.Fatalf("expected value 1 from OnCalculated, got %s", m.Value)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out reading from OnCalculated channel")
	}

	bs.alertCh <- decimal.NewFromInt(2)
	select {
	case v := <-alertCh:
		if !v.Equal(decimal.NewFromInt(2)) {
			t.Fatalf("expected value 2 from OnAlertTriggered, got %s", v)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out reading from OnAlertTriggered channel")
	}
}

// TestBaseStudy_ConcurrentAddCalculation verifies that concurrent goroutines
// adding calculations does not cause races or panics.
func TestBaseStudy_ConcurrentAddCalculation(t *testing.T) {
	bs := newTestStudy()
	bs.SetAggregationLevel(enums.AggregationNone)
	bs.SetStaleDuration(10 * time.Minute)

	ctx := context.Background()
	if err := bs.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer bs.StopAsync(ctx)

	const numGoroutines = 10
	const itemsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < itemsPerGoroutine; i++ {
				bs.AddCalculation(models.BaseStudyModel{
					Value:     decimal.NewFromInt(int64(id*1000 + i)),
					Timestamp: time.Now(),
				})
			}
		}(g)
	}

	// Drain output channel in a separate goroutine.
	received := make(chan int, 1)
	go func() {
		count := 0
		for count < numGoroutines*itemsPerGoroutine {
			select {
			case <-bs.OnCalculated():
				count++
			case <-time.After(5 * time.Second):
				received <- count
				return
			}
		}
		received <- count
	}()

	wg.Wait()

	count := <-received
	if count == 0 {
		t.Fatal("no items received from concurrent AddCalculation")
	}
	t.Logf("received %d/%d items from concurrent producers", count, numGoroutines*itemsPerGoroutine)
}
