package connector

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/core/enums"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

const defaultStaleThreshold = 30 * time.Second

// StaleDetector monitors provider LastUpdated timestamps and marks providers
// as ConnectedWithWarnings if they have not sent data within the stale
// threshold (default 30 seconds). It subscribes to the Providers topic on the
// event bus to track provider state and publishes updated status when
// staleness is detected.
type StaleDetector struct {
	bus            *eventbus.Bus
	logger         *slog.Logger
	staleThreshold time.Duration

	mu        sync.RWMutex
	providers map[int]models.Provider // providerID -> latest provider
}

// StaleDetectorOption configures a StaleDetector.
type StaleDetectorOption func(*StaleDetector)

// WithStaleThreshold overrides the default 30-second stale threshold.
func WithStaleThreshold(d time.Duration) StaleDetectorOption {
	return func(sd *StaleDetector) {
		sd.staleThreshold = d
	}
}

// NewStaleDetector creates a new StaleDetector that tracks provider freshness
// via the event bus.
func NewStaleDetector(bus *eventbus.Bus, logger *slog.Logger, opts ...StaleDetectorOption) *StaleDetector {
	if logger == nil {
		logger = slog.Default()
	}
	sd := &StaleDetector{
		bus:            bus,
		logger:         logger,
		staleThreshold: defaultStaleThreshold,
		providers:      make(map[int]models.Provider),
	}
	for _, opt := range opts {
		opt(sd)
	}
	return sd
}

// Run starts the stale detector. It subscribes to the Providers topic to
// track provider state and runs a ticker that checks for stale providers.
// It blocks until ctx is cancelled.
func (sd *StaleDetector) Run(ctx context.Context) {
	// Subscribe to provider updates.
	subID, provCh := sd.bus.Providers.Subscribe(32)
	defer sd.bus.Providers.Unsubscribe(subID)

	ticker := time.NewTicker(sd.staleThreshold)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case p, ok := <-provCh:
			if !ok {
				return
			}
			sd.mu.Lock()
			sd.providers[p.ProviderID] = p
			sd.mu.Unlock()

		case <-ticker.C:
			sd.checkStale()
		}
	}
}

// checkStale iterates over tracked providers and publishes a
// ConnectedWithWarnings status for any provider whose LastUpdated is older
// than the stale threshold. Only providers that are currently Connected are
// marked as stale.
func (sd *StaleDetector) checkStale() {
	now := time.Now()

	sd.mu.Lock()
	defer sd.mu.Unlock()

	for id, p := range sd.providers {
		if p.Status != enums.SessionConnected {
			continue
		}

		if now.Sub(p.LastUpdated) > sd.staleThreshold {
			sd.logger.Warn("provider stale, marking as ConnectedWithWarnings",
				slog.Int("providerID", p.ProviderID),
				slog.String("providerName", p.ProviderName),
				slog.Duration("staleDuration", now.Sub(p.LastUpdated)),
			)

			updated := p
			updated.Status = enums.SessionConnectedWithWarnings
			updated.LastUpdated = now

			sd.providers[id] = updated
			sd.bus.Providers.Publish(updated)
		}
	}
}
