package connector

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/core/enums"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

// ---------------------------------------------------------------------------
// 1. TestStaleDetector_DetectsStaleProvider
// ---------------------------------------------------------------------------

func TestStaleDetector_DetectsStaleProvider(t *testing.T) {
	logger := slog.Default()
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	// Use a very short threshold for testing.
	threshold := 100 * time.Millisecond

	sd := NewStaleDetector(bus, logger, WithStaleThreshold(threshold))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Seed the provider directly into the detector's map before starting
	// the Run loop so there is no race between consuming the provider event
	// and the first ticker fire.
	staleTime := time.Now().Add(-2 * threshold)
	sd.mu.Lock()
	sd.providers[1] = models.Provider{
		ProviderID:   1,
		ProviderName: "StaleExchange",
		Status:       enums.SessionConnected,
		LastUpdated:  staleTime,
	}
	sd.mu.Unlock()

	// Subscribe to provider events to observe stale detection output.
	_, provCh := bus.Providers.Subscribe(32)

	go sd.Run(ctx)

	// Wait for the detector to tick and publish a stale warning.
	deadline := time.After(5 * time.Second)
	var gotWarning bool
	for !gotWarning {
		select {
		case p := <-provCh:
			if p.ProviderID == 1 && p.Status == enums.SessionConnectedWithWarnings {
				gotWarning = true
			}
		case <-deadline:
			t.Fatal("timed out waiting for stale detection")
		}
	}

	if !gotWarning {
		t.Error("expected provider to be marked as ConnectedWithWarnings")
	}
}

// ---------------------------------------------------------------------------
// 2. TestStaleDetector_NonStaleProviderLeftAlone
// ---------------------------------------------------------------------------

func TestStaleDetector_NonStaleProviderLeftAlone(t *testing.T) {
	logger := slog.Default()
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	threshold := 100 * time.Millisecond

	sd := NewStaleDetector(bus, logger, WithStaleThreshold(threshold))

	// Subscribe to provider events.
	_, provCh := bus.Providers.Subscribe(32)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sd.Run(ctx)

	// Publish a fresh (non-stale) provider.
	bus.Providers.Publish(models.Provider{
		ProviderID:   2,
		ProviderName: "FreshExchange",
		Status:       enums.SessionConnected,
		LastUpdated:  time.Now(),
	})

	// Drain the initial publish event.
	select {
	case <-provCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for initial provider event")
	}

	// Wait a bit longer than a tick but shorter than the stale threshold.
	// The provider was just published with time.Now(), so after one tick
	// (threshold), it should still be fresh because time.Now() - lastUpdated
	// will be approximately equal to threshold (borderline). We use a short
	// wait and check no warning was emitted.
	select {
	case p := <-provCh:
		// If we get an event, it should NOT be a ConnectedWithWarnings for provider 2.
		if p.ProviderID == 2 && p.Status == enums.SessionConnectedWithWarnings {
			t.Error("non-stale provider should not be marked as ConnectedWithWarnings")
		}
	case <-time.After(threshold + 50*time.Millisecond):
		// No event is the expected behavior for a fresh provider. However,
		// depending on timing, the provider might become borderline stale.
		// This is acceptable -- the important thing is we exercised the path.
	}
}

// ---------------------------------------------------------------------------
// 3. TestStaleDetector_ContextCancellationStops
// ---------------------------------------------------------------------------

func TestStaleDetector_ContextCancellationStops(t *testing.T) {
	logger := slog.Default()
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	threshold := 50 * time.Millisecond
	sd := NewStaleDetector(bus, logger, WithStaleThreshold(threshold))

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		sd.Run(ctx)
		close(done)
	}()

	// Cancel the context.
	cancel()

	// The Run goroutine should exit promptly.
	select {
	case <-done:
		// Success: Run returned after context cancellation.
	case <-time.After(2 * time.Second):
		t.Fatal("StaleDetector.Run did not stop after context cancellation")
	}
}

// ---------------------------------------------------------------------------
// 4. TestStaleDetector_OnlyConnectedProvidersChecked
// ---------------------------------------------------------------------------

func TestStaleDetector_OnlyConnectedProvidersChecked(t *testing.T) {
	logger := slog.Default()
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	threshold := 50 * time.Millisecond

	sd := NewStaleDetector(bus, logger, WithStaleThreshold(threshold))

	_, provCh := bus.Providers.Subscribe(32)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sd.Run(ctx)

	// Publish a provider that is Disconnected and stale -- it should NOT be
	// marked with ConnectedWithWarnings.
	staleTime := time.Now().Add(-2 * threshold)
	bus.Providers.Publish(models.Provider{
		ProviderID:   3,
		ProviderName: "DisconnectedExchange",
		Status:       enums.SessionDisconnected,
		LastUpdated:  staleTime,
	})

	// Drain the initial publish.
	select {
	case <-provCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for initial provider event")
	}

	// Wait for a couple of ticks and verify no ConnectedWithWarnings event.
	timeout := time.After(3 * threshold)
	for {
		select {
		case p := <-provCh:
			if p.ProviderID == 3 && p.Status == enums.SessionConnectedWithWarnings {
				t.Fatal("disconnected provider should not be marked as ConnectedWithWarnings")
			}
		case <-timeout:
			// No warning event for the disconnected provider -- correct.
			return
		}
	}
}
