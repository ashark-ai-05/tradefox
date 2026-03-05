package plugin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/core/enums"
)

// discardLogger returns a logger that writes to nowhere, suitable for tests.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(devNull{}, nil))
}

type devNull struct{}

func (devNull) Write(p []byte) (int, error) { return len(p), nil }

// ---------------------------------------------------------------------------
// TestSupervisor_CleanExit
// ---------------------------------------------------------------------------

func TestSupervisor_CleanExit(t *testing.T) {
	called := false
	runFn := func(ctx context.Context) error {
		called = true
		return nil
	}

	sv := NewSupervisor("test-clean", runFn, discardLogger())
	err := sv.Run(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if !called {
		t.Fatal("expected runFn to be called")
	}
}

// ---------------------------------------------------------------------------
// TestSupervisor_PanicRecovery
// ---------------------------------------------------------------------------

func TestSupervisor_PanicRecovery(t *testing.T) {
	var mu sync.Mutex
	callCount := 0

	runFn := func(ctx context.Context) error {
		mu.Lock()
		callCount++
		n := callCount
		mu.Unlock()

		if n <= 2 {
			panic(fmt.Sprintf("panic #%d", n))
		}
		return nil // succeed on third call
	}

	sv := NewSupervisor("test-panic", runFn, discardLogger())
	sv.SetMaxRetries(5)

	err := sv.Run(context.Background())
	if err != nil {
		t.Fatalf("expected nil error after recovery, got: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if callCount != 3 {
		t.Fatalf("expected 3 calls (2 panics + 1 success), got %d", callCount)
	}
}

// ---------------------------------------------------------------------------
// TestSupervisor_ErrorRecovery
// ---------------------------------------------------------------------------

func TestSupervisor_ErrorRecovery(t *testing.T) {
	var mu sync.Mutex
	callCount := 0

	runFn := func(ctx context.Context) error {
		mu.Lock()
		callCount++
		n := callCount
		mu.Unlock()

		if n <= 2 {
			return fmt.Errorf("error #%d", n)
		}
		return nil
	}

	sv := NewSupervisor("test-error", runFn, discardLogger())
	sv.SetMaxRetries(5)

	err := sv.Run(context.Background())
	if err != nil {
		t.Fatalf("expected nil error after recovery, got: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if callCount != 3 {
		t.Fatalf("expected 3 calls (2 errors + 1 success), got %d", callCount)
	}
}

// ---------------------------------------------------------------------------
// TestSupervisor_MaxRetriesExceeded
// ---------------------------------------------------------------------------

func TestSupervisor_MaxRetriesExceeded(t *testing.T) {
	callCount := 0
	runFn := func(ctx context.Context) error {
		callCount++
		panic("always panics")
	}

	var mu sync.Mutex
	var statuses []enums.PluginStatus

	sv := NewSupervisor("test-max-retries", runFn, discardLogger())
	sv.SetMaxRetries(3)
	sv.SetStatusCallback(func(status enums.PluginStatus) {
		mu.Lock()
		statuses = append(statuses, status)
		mu.Unlock()
	})

	err := sv.Run(context.Background())
	if err == nil {
		t.Fatal("expected non-nil error when max retries exceeded")
	}

	if callCount != 3 {
		t.Fatalf("expected 3 calls, got %d", callCount)
	}

	mu.Lock()
	defer mu.Unlock()

	// The last status should be StoppedFailed.
	if len(statuses) == 0 {
		t.Fatal("expected at least one status callback")
	}
	lastStatus := statuses[len(statuses)-1]
	if lastStatus != enums.PluginStoppedFailed {
		t.Fatalf("expected last status PluginStoppedFailed, got %v", lastStatus)
	}
}

// ---------------------------------------------------------------------------
// TestSupervisor_ContextCancellation
// ---------------------------------------------------------------------------

func TestSupervisor_ContextCancellation(t *testing.T) {
	// This test verifies that if the context is cancelled during the backoff
	// wait, the supervisor returns nil promptly.

	callCount := 0
	runFn := func(ctx context.Context) error {
		callCount++
		return errors.New("fail to trigger backoff")
	}

	sv := NewSupervisor("test-ctx-cancel", runFn, discardLogger())
	sv.SetMaxRetries(10) // high so we don't hit the limit

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- sv.Run(ctx)
	}()

	// Wait enough for at least one retry + start of backoff, then cancel.
	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil error on context cancellation, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not stop within 5 seconds after context cancellation")
	}

	if callCount < 1 {
		t.Fatal("expected at least one call before cancellation")
	}
}

// ---------------------------------------------------------------------------
// TestSupervisor_StatusCallbacks
// ---------------------------------------------------------------------------

func TestSupervisor_StatusCallbacks(t *testing.T) {
	var mu sync.Mutex
	callCount := 0
	var statuses []enums.PluginStatus

	runFn := func(ctx context.Context) error {
		mu.Lock()
		callCount++
		n := callCount
		mu.Unlock()

		if n <= 2 {
			return fmt.Errorf("fail #%d", n)
		}
		return nil
	}

	sv := NewSupervisor("test-status", runFn, discardLogger())
	sv.SetMaxRetries(5)
	sv.SetStatusCallback(func(status enums.PluginStatus) {
		mu.Lock()
		statuses = append(statuses, status)
		mu.Unlock()
	})

	err := sv.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	// After first failure (attempt=1, not at max): PluginStarting
	// After second failure (attempt=2, not at max): PluginStarting
	// Third call succeeds: no status change.
	expected := []enums.PluginStatus{enums.PluginStarting, enums.PluginStarting}
	if len(statuses) != len(expected) {
		t.Fatalf("expected %d status callbacks, got %d: %v", len(expected), len(statuses), statuses)
	}
	for i, s := range expected {
		if statuses[i] != s {
			t.Errorf("status[%d]: expected %v, got %v", i, s, statuses[i])
		}
	}
}

// ---------------------------------------------------------------------------
// TestSupervisor_ExponentialBackoff
// ---------------------------------------------------------------------------

func TestSupervisor_ExponentialBackoff(t *testing.T) {
	// We measure the wall-clock time between successive calls to runFn.
	// The backoff for attempt N is 2^N seconds + up to 1s jitter.
	// attempt=1 -> ~2s (+jitter 0-1s) = 2-3s
	// attempt=2 -> ~4s (+jitter 0-1s) = 4-5s
	//
	// To keep the test fast, we use maxRetries=3 (so only 3 calls total,
	// measuring 2 backoff intervals) and verify durations are in range.

	var mu sync.Mutex
	var callTimes []time.Time
	callCount := 0

	runFn := func(ctx context.Context) error {
		mu.Lock()
		callTimes = append(callTimes, time.Now())
		callCount++
		mu.Unlock()
		return errors.New("fail")
	}

	sv := NewSupervisor("test-backoff", runFn, discardLogger())
	sv.SetMaxRetries(3)

	start := time.Now()
	_ = sv.Run(context.Background())
	totalDuration := time.Since(start)

	mu.Lock()
	defer mu.Unlock()

	if len(callTimes) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(callTimes))
	}

	// First backoff (attempt 1): 2^1 = 2s base, so 2s-3s expected.
	gap1 := callTimes[1].Sub(callTimes[0])
	if gap1 < 1900*time.Millisecond || gap1 > 3200*time.Millisecond {
		t.Errorf("first backoff gap: expected ~2-3s, got %v", gap1)
	}

	// Second backoff (attempt 2): 2^2 = 4s base, so 4s-5s expected.
	gap2 := callTimes[2].Sub(callTimes[1])
	if gap2 < 3900*time.Millisecond || gap2 > 5200*time.Millisecond {
		t.Errorf("second backoff gap: expected ~4-5s, got %v", gap2)
	}

	// Verify the second backoff is longer than the first (exponential increase).
	if gap2 <= gap1 {
		t.Errorf("expected second backoff (%v) > first backoff (%v)", gap2, gap1)
	}

	// Total should be roughly 6-8s (2-3 + 4-5).
	if totalDuration < 5*time.Second || totalDuration > 9*time.Second {
		t.Errorf("total duration: expected ~6-8s, got %v", totalDuration)
	}
}
