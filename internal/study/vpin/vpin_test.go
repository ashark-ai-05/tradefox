package vpin

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/ashark-ai-05/tradefox/internal/config"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

func testLogger() *slog.Logger {
	return slog.Default()
}

func newTestBus() *eventbus.Bus {
	return eventbus.NewBus(testLogger())
}

func boolPtr(b bool) *bool {
	return &b
}

// makeTrade creates a Trade with the given size and side.
func makeTrade(size float64, isBuy bool) models.Trade {
	return models.Trade{
		Size:      decimal.NewFromFloat(size),
		IsBuy:     boolPtr(isBuy),
		Timestamp: time.Now(),
	}
}

// newTestVPIN creates a VPINStudy with the given bucket volume size, starts
// it, and returns the study plus a cleanup function.
func newTestVPIN(t *testing.T, bucketSize float64) (*VPINStudy, context.CancelFunc) {
	t.Helper()
	bus := newTestBus()
	settings := config.NewManager("")
	v := New(bus, settings, testLogger())
	v.SetBucketVolumeSize(decimal.NewFromFloat(bucketSize))

	ctx, cancel := context.WithCancel(context.Background())
	if err := v.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}

	return v, func() {
		v.StopAsync(ctx)
		cancel()
	}
}

// drainOutput reads from the output channel until timeout, returning all
// received models.
func drainOutput(ch <-chan models.BaseStudyModel, count int, timeout time.Duration) []models.BaseStudyModel {
	var results []models.BaseStudyModel
	deadline := time.After(timeout)
	for i := 0; i < count; i++ {
		select {
		case m := <-ch:
			results = append(results, m)
		case <-deadline:
			return results
		}
	}
	return results
}

// TestVPIN_AllBuys verifies that 10 buy trades filling exactly one bucket
// produce VPIN = 1.0 (complete imbalance).
func TestVPIN_AllBuys(t *testing.T) {
	v, cleanup := newTestVPIN(t, 100)
	defer cleanup()

	outCh := v.OnCalculated()

	// 10 buy trades of size 10 each = total 100, exactly filling one bucket.
	for i := 0; i < 10; i++ {
		v.ProcessTrade(makeTrade(10, true))
	}

	// The last trade completes the bucket (100 == 100), so it is an interim
	// update (no overflow). We should get 10 interim outputs, all with VPIN = 1.0.
	results := drainOutput(outCh, 10, 2*time.Second)
	if len(results) == 0 {
		t.Fatal("expected at least one output, got none")
	}

	last := results[len(results)-1]
	expected := decimal.NewFromInt(1)
	if !last.Value.Equal(expected) {
		t.Fatalf("expected VPIN = 1.0 for all-buy trades, got %s", last.Value)
	}
}

// TestVPIN_BalancedBuySell verifies that 5 buy + 5 sell trades of equal
// size produce VPIN = 0.0 (perfect balance).
func TestVPIN_BalancedBuySell(t *testing.T) {
	v, cleanup := newTestVPIN(t, 100)
	defer cleanup()

	outCh := v.OnCalculated()

	// 5 buy trades of size 10.
	for i := 0; i < 5; i++ {
		v.ProcessTrade(makeTrade(10, true))
	}
	// 5 sell trades of size 10.
	for i := 0; i < 5; i++ {
		v.ProcessTrade(makeTrade(10, false))
	}

	// Drain all 10 outputs.
	results := drainOutput(outCh, 10, 2*time.Second)
	if len(results) == 0 {
		t.Fatal("expected at least one output, got none")
	}

	last := results[len(results)-1]
	expected := decimal.Zero
	if !last.Value.Equal(expected) {
		t.Fatalf("expected VPIN = 0.0 for balanced buy/sell, got %s", last.Value)
	}
}

// TestVPIN_BucketOverflow verifies that when a trade's volume causes the
// bucket to overflow, the bucket is completed and the excess carries over
// into the next bucket.
func TestVPIN_BucketOverflow(t *testing.T) {
	v, cleanup := newTestVPIN(t, 100)
	defer cleanup()

	outCh := v.OnCalculated()

	// Fill bucket to 80 with buy volume.
	v.ProcessTrade(makeTrade(80, true))
	// Drain the interim output.
	drainOutput(outCh, 1, time.Second)

	// Now add a sell trade of size 40 which overflows the bucket:
	// - 20 goes to current bucket (completing it at 100: buy=80, sell=20)
	// - 20 carries over to the next bucket as sell volume.
	v.ProcessTrade(makeTrade(40, false))

	// We should get two outputs from this trade:
	// 1. The completed bucket (isNewBucket=true, AddItemSkippingAggregation=true)
	//    VPIN = |80 - 20| / (80 + 20) = 60/100 = 0.6
	results := drainOutput(outCh, 1, 2*time.Second)
	if len(results) == 0 {
		t.Fatal("expected output for completed bucket, got none")
	}

	completedBucket := results[0]
	expectedVPIN := decimal.NewFromFloat(0.6)
	if !completedBucket.Value.Equal(expectedVPIN) {
		t.Fatalf("expected completed bucket VPIN = 0.6, got %s", completedBucket.Value)
	}
	if !completedBucket.AddItemSkippingAggregation {
		t.Fatal("expected AddItemSkippingAggregation=true for completed bucket")
	}
	if completedBucket.ValueColor != colorGreen {
		t.Fatalf("expected ValueColor='Green' for completed bucket, got %q", completedBucket.ValueColor)
	}

	// Verify the carry-over state: next bucket should have sell=20, buy=0,
	// total bucket vol=20. Feed one more small buy trade to observe the state.
	v.ProcessTrade(makeTrade(10, true))
	results2 := drainOutput(outCh, 1, 2*time.Second)
	if len(results2) == 0 {
		t.Fatal("expected output after carry-over trade, got none")
	}

	// After carry-over: buy=10, sell=20 => VPIN = |10-20|/(10+20) = 10/30 = 0.333...
	afterCarryOver := results2[0]
	expectedAfter := decimal.NewFromFloat(10).Div(decimal.NewFromFloat(30))
	if !afterCarryOver.Value.Equal(expectedAfter) {
		t.Fatalf("expected carry-over VPIN = %s, got %s", expectedAfter, afterCarryOver.Value)
	}
}

// TestVPIN_InterimVsCompletion verifies that interim updates (no bucket
// completion) differ from bucket completion updates.
func TestVPIN_InterimVsCompletion(t *testing.T) {
	v, cleanup := newTestVPIN(t, 100)
	defer cleanup()

	outCh := v.OnCalculated()

	// Trade that does not complete the bucket (interim update).
	v.ProcessTrade(makeTrade(50, true))
	results := drainOutput(outCh, 1, 2*time.Second)
	if len(results) == 0 {
		t.Fatal("expected interim output, got none")
	}

	interim := results[0]
	if interim.AddItemSkippingAggregation {
		t.Fatal("interim update should have AddItemSkippingAggregation=false")
	}
	if interim.ValueColor != colorWhite {
		t.Fatalf("interim update should have ValueColor='White', got %q", interim.ValueColor)
	}
	// VPIN for buy=50, sell=0 => |50-0|/50 = 1.0
	if !interim.Value.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("expected interim VPIN = 1.0, got %s", interim.Value)
	}

	// Trade that overflows the bucket (bucket completion).
	v.ProcessTrade(makeTrade(60, false))
	results2 := drainOutput(outCh, 1, 2*time.Second)
	if len(results2) == 0 {
		t.Fatal("expected completion output, got none")
	}

	completion := results2[0]
	if !completion.AddItemSkippingAggregation {
		t.Fatal("bucket completion should have AddItemSkippingAggregation=true")
	}
	if completion.ValueColor != colorGreen {
		t.Fatalf("bucket completion should have ValueColor='Green', got %q", completion.ValueColor)
	}
	// Bucket at completion: buy=50, sell=50, VPIN = |50-50|/100 = 0.0
	if !completion.Value.Equal(decimal.Zero) {
		t.Fatalf("expected completion VPIN = 0.0, got %s", completion.Value)
	}
}

// TestVPIN_ConstructorMetadata verifies that the study is constructed with
// the correct metadata and default configuration.
func TestVPIN_ConstructorMetadata(t *testing.T) {
	bus := newTestBus()
	settings := config.NewManager("")
	v := New(bus, settings, testLogger())

	if v.Name() != "VPIN" {
		t.Fatalf("expected Name 'VPIN', got %q", v.Name())
	}
	if v.Version() != "1.0.0" {
		t.Fatalf("expected Version '1.0.0', got %q", v.Version())
	}
	if v.Description() != "Volume-Synchronized Probability of Informed Trading" {
		t.Fatalf("unexpected Description: %q", v.Description())
	}
	if v.Author() != "VisualHFT" {
		t.Fatalf("expected Author 'VisualHFT', got %q", v.Author())
	}
	if v.TileTitle() != "VPIN" {
		t.Fatalf("expected TileTitle 'VPIN', got %q", v.TileTitle())
	}
	if v.TileToolTip() != "Volume-Synchronized Probability of Informed Trading" {
		t.Fatalf("expected TileToolTip, got %q", v.TileToolTip())
	}
	expectedBucket := decimal.NewFromFloat(100)
	if !v.BucketVolumeSize().Equal(expectedBucket) {
		t.Fatalf("expected default BucketVolumeSize 100, got %s", v.BucketVolumeSize())
	}
	if v.PluginUniqueID() == "" {
		t.Fatal("expected non-empty PluginUniqueID")
	}
}

// TestVPIN_NilIsBuyIgnored verifies that trades with nil IsBuy are silently
// ignored and produce no output.
func TestVPIN_NilIsBuyIgnored(t *testing.T) {
	v, cleanup := newTestVPIN(t, 100)
	defer cleanup()

	outCh := v.OnCalculated()

	// Send a trade with nil IsBuy.
	v.ProcessTrade(models.Trade{
		Size:      decimal.NewFromFloat(10),
		IsBuy:     nil,
		Timestamp: time.Now(),
	})

	// No output should be produced.
	select {
	case m := <-outCh:
		t.Fatalf("expected no output for nil IsBuy trade, got %+v", m)
	case <-time.After(200 * time.Millisecond):
		// Expected: no output.
	}
}

// TestVPIN_EventBusIntegration verifies that trades published on the event
// bus are processed by the study and VPIN values appear on both the output
// channel and the Studies topic.
func TestVPIN_EventBusIntegration(t *testing.T) {
	bus := newTestBus()
	settings := config.NewManager("")
	v := New(bus, settings, testLogger())
	v.SetBucketVolumeSize(decimal.NewFromFloat(50))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := v.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer v.StopAsync(ctx)

	// Subscribe to the Studies topic to verify publication.
	_, studiesCh := bus.Studies.Subscribe(64)

	// Publish a trade on the bus.
	bus.Trades.Publish(models.Trade{
		Size:      decimal.NewFromFloat(25),
		IsBuy:     boolPtr(true),
		Timestamp: time.Now(),
	})

	// We should receive the VPIN result on the Studies topic.
	select {
	case m := <-studiesCh:
		// buy=25, sell=0 => VPIN = 1.0
		if !m.Value.Equal(decimal.NewFromInt(1)) {
			t.Fatalf("expected VPIN = 1.0 from Studies topic, got %s", m.Value)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for VPIN on Studies topic")
	}
}

// TestVPIN_MultipleOverflows verifies correct behavior across multiple
// consecutive bucket completions when a large trade spans multiple buckets.
func TestVPIN_MultipleOverflows(t *testing.T) {
	v, cleanup := newTestVPIN(t, 50)
	defer cleanup()

	outCh := v.OnCalculated()

	// Fill bucket with 30 buy.
	v.ProcessTrade(makeTrade(30, true))
	drainOutput(outCh, 1, time.Second) // interim

	// Add 40 sell, overflows by 20 (bucket completes at 50: buy=30, sell=20).
	v.ProcessTrade(makeTrade(40, false))
	results := drainOutput(outCh, 1, 2*time.Second)
	if len(results) == 0 {
		t.Fatal("expected bucket completion output")
	}

	// VPIN = |30 - 20| / (30 + 20) = 10/50 = 0.2
	expectedVPIN := decimal.NewFromFloat(0.2)
	if !results[0].Value.Equal(expectedVPIN) {
		t.Fatalf("expected VPIN = 0.2, got %s", results[0].Value)
	}

	// Carry-over: sell=20, bucket vol=20.
	// Add another 40 buy, overflows again (20+40=60 > 50, overflow=10).
	// buy portion in bucket = 40-10 = 30, sell=20 in this bucket.
	v.ProcessTrade(makeTrade(40, true))
	results2 := drainOutput(outCh, 1, 2*time.Second)
	if len(results2) == 0 {
		t.Fatal("expected second bucket completion output")
	}

	// VPIN = |30 - 20| / (30 + 20) = 10/50 = 0.2
	if !results2[0].Value.Equal(expectedVPIN) {
		t.Fatalf("expected second bucket VPIN = 0.2, got %s", results2[0].Value)
	}

	// Carry-over: buy=10, sell=0, bucket vol=10.
	// Verify by sending a small sell trade.
	v.ProcessTrade(makeTrade(5, false))
	results3 := drainOutput(outCh, 1, 2*time.Second)
	if len(results3) == 0 {
		t.Fatal("expected interim output after second carry-over")
	}

	// buy=10, sell=5 => VPIN = |10-5|/(10+5) = 5/15 = 0.333...
	expectedAfter := decimal.NewFromFloat(5).Div(decimal.NewFromFloat(15))
	if !results3[0].Value.Equal(expectedAfter) {
		t.Fatalf("expected VPIN = %s after second carry-over, got %s", expectedAfter, results3[0].Value)
	}
}
