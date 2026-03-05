// Package study provides the base implementation for all VisualHFT analytics
// studies (VPIN, LOB Imbalance, etc.). BaseStudy implements the Study and
// Plugin interfaces and provides time-windowed aggregation, stale detection,
// provider status monitoring, and automatic restart with exponential backoff.
package study

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shopspring/decimal"
	"github.com/ashark-ai-05/tradefox/internal/config"
	"github.com/ashark-ai-05/tradefox/internal/core/enums"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

// BaseStudy provides the foundation for all analytics studies. Derived studies
// embed *BaseStudy and override the OnDataAggregation hook to provide custom
// aggregation logic.
type BaseStudy struct {
	// Plugin metadata (set by derived studies via constructor or setters).
	name        string
	version     string
	description string
	author      string
	tileTitle   string
	tileToolTip string

	status         atomic.Value // stores enums.PluginStatus
	pluginUniqueID string

	// Provider association.
	providerID   int
	providerName string

	// Aggregation configuration.
	aggLevel enums.AggregationLevel

	// Dependencies.
	bus      *eventbus.Bus
	settings *config.Manager
	logger   *slog.Logger

	// Internal queue and output channels.
	calcCh   chan models.BaseStudyModel
	outputCh chan models.BaseStudyModel
	alertCh  chan decimal.Decimal

	// Aggregation state (accessed only from the consumer goroutine).
	lastBucketTime time.Time
	currentBucket  *models.BaseStudyModel
	bucketCount    int

	// Stale detection.
	staleTimer    *time.Timer
	staleDuration time.Duration
	staleMu       sync.Mutex

	// Restart logic.
	restartSem         chan struct{}
	restartAttempt     int
	maxRestartAttempts int
	restartMu          sync.Mutex

	// Lifecycle.
	cancel context.CancelFunc
	done   chan struct{}

	// Provider monitoring.
	providerSubID uint64

	// Override hooks (set by derived studies).
	// OnDataAggregation is called when a new item falls within the same time
	// bucket as an existing aggregated item. The derived study can merge values
	// as needed. existing is a pointer to the current bucket; newItem is the
	// incoming data point; count is the number of items aggregated so far
	// (including the new one).
	OnDataAggregation func(existing *models.BaseStudyModel, newItem models.BaseStudyModel, count int)
}

// NewBaseStudy creates a new BaseStudy with the given metadata and dependencies.
// All channels are initialized and the status is set to PluginLoaded.
func NewBaseStudy(
	name, version, description, author string,
	bus *eventbus.Bus,
	settings *config.Manager,
	logger *slog.Logger,
) *BaseStudy {
	bs := &BaseStudy{
		name:               name,
		version:            version,
		description:        description,
		author:             author,
		bus:                bus,
		settings:           settings,
		logger:             logger,
		calcCh:             make(chan models.BaseStudyModel, 10000),
		outputCh:           make(chan models.BaseStudyModel, 1000),
		alertCh:            make(chan decimal.Decimal, 100),
		staleDuration:      30 * time.Second,
		maxRestartAttempts: 5,
		restartSem:         make(chan struct{}, 1),
	}
	bs.status.Store(enums.PluginLoaded)
	bs.pluginUniqueID = bs.computeUniqueID()
	return bs
}

// ---------------------------------------------------------------------------
// Plugin interface
// ---------------------------------------------------------------------------

// Name returns the study name.
func (bs *BaseStudy) Name() string { return bs.name }

// Version returns the study version.
func (bs *BaseStudy) Version() string { return bs.version }

// Description returns the study description.
func (bs *BaseStudy) Description() string { return bs.description }

// Author returns the study author.
func (bs *BaseStudy) Author() string { return bs.author }

// PluginType returns PluginTypeStudy.
func (bs *BaseStudy) PluginType() enums.PluginType { return enums.PluginTypeStudy }

// PluginUniqueID returns the SHA256-based unique identifier.
func (bs *BaseStudy) PluginUniqueID() string { return bs.pluginUniqueID }

// RequiredLicenseLevel defaults to LicenseCommunity.
func (bs *BaseStudy) RequiredLicenseLevel() enums.LicenseLevel { return enums.LicenseCommunity }

// Status returns the current plugin status.
func (bs *BaseStudy) Status() enums.PluginStatus {
	v := bs.status.Load()
	if v == nil {
		return enums.PluginLoaded
	}
	return v.(enums.PluginStatus)
}

// SetStatus atomically stores a new plugin status.
func (bs *BaseStudy) SetStatus(s enums.PluginStatus) {
	bs.status.Store(s)
}

// ---------------------------------------------------------------------------
// Tile metadata
// ---------------------------------------------------------------------------

// TileTitle returns the display title for the study tile.
func (bs *BaseStudy) TileTitle() string { return bs.tileTitle }

// TileToolTip returns the tooltip text for the study tile.
func (bs *BaseStudy) TileToolTip() string { return bs.tileToolTip }

// SetTileTitle sets the tile display title.
func (bs *BaseStudy) SetTileTitle(t string) { bs.tileTitle = t }

// SetTileToolTip sets the tile tooltip text.
func (bs *BaseStudy) SetTileToolTip(t string) { bs.tileToolTip = t }

// ---------------------------------------------------------------------------
// Output channels
// ---------------------------------------------------------------------------

// OnCalculated returns a receive-only channel that emits calculated study values.
func (bs *BaseStudy) OnCalculated() <-chan models.BaseStudyModel { return bs.outputCh }

// OnAlertTriggered returns a receive-only channel that emits alert values.
func (bs *BaseStudy) OnAlertTriggered() <-chan decimal.Decimal { return bs.alertCh }

// ---------------------------------------------------------------------------
// Configuration helpers
// ---------------------------------------------------------------------------

// SetProviderID sets the associated provider ID.
func (bs *BaseStudy) SetProviderID(id int) { bs.providerID = id }

// SetProviderName sets the associated provider name.
func (bs *BaseStudy) SetProviderName(name string) { bs.providerName = name }

// SetAggregationLevel sets the time-based aggregation granularity.
func (bs *BaseStudy) SetAggregationLevel(level enums.AggregationLevel) { bs.aggLevel = level }

// SetStaleDuration overrides the default stale detection timeout (30s).
// Must be called before StartAsync.
func (bs *BaseStudy) SetStaleDuration(d time.Duration) { bs.staleDuration = d }

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// StartAsync initializes the study's internal processing pipeline and starts
// the consumer goroutine that reads from the calculation channel.
func (bs *BaseStudy) StartAsync(ctx context.Context) error {
	bs.SetStatus(enums.PluginStarting)
	bs.logger.Info("study starting",
		slog.String("name", bs.name),
		slog.String("aggregation", bs.aggLevel.String()),
	)

	// Reset aggregation state.
	bs.lastBucketTime = time.Time{}
	bs.currentBucket = nil
	bs.bucketCount = 0

	// Create cancellable context for goroutines.
	ctx, cancel := context.WithCancel(ctx)
	bs.cancel = cancel
	bs.done = make(chan struct{})

	// Start stale detection timer.
	bs.staleMu.Lock()
	bs.staleTimer = time.NewTimer(bs.staleDuration)
	bs.staleMu.Unlock()

	// Subscribe to provider status changes.
	if bs.bus != nil {
		bs.providerSubID, _ = bs.bus.Providers.Subscribe(64)
	}

	// Start the consumer goroutine.
	go bs.consumeLoop(ctx)

	bs.SetStatus(enums.PluginStarted)
	bs.logger.Info("study started", slog.String("name", bs.name))
	return nil
}

// StopAsync stops the study and closes internal channels.
func (bs *BaseStudy) StopAsync(_ context.Context) error {
	bs.SetStatus(enums.PluginStopping)
	bs.logger.Info("study stopping", slog.String("name", bs.name))

	if bs.cancel != nil {
		bs.cancel()
	}

	// Wait for consumer goroutine to finish.
	if bs.done != nil {
		<-bs.done
	}

	// Stop stale timer.
	bs.staleMu.Lock()
	if bs.staleTimer != nil {
		bs.staleTimer.Stop()
		bs.staleTimer = nil
	}
	bs.staleMu.Unlock()

	// Unsubscribe from provider topic.
	if bs.bus != nil && bs.providerSubID > 0 {
		bs.bus.Providers.Unsubscribe(bs.providerSubID)
		bs.providerSubID = 0
	}

	bs.SetStatus(enums.PluginStopped)
	bs.logger.Info("study stopped", slog.String("name", bs.name))
	return nil
}

// ---------------------------------------------------------------------------
// Internal queue
// ---------------------------------------------------------------------------

// AddCalculation adds a study model to the internal processing queue.
// It is non-blocking: if the queue is full, the item is dropped.
func (bs *BaseStudy) AddCalculation(model models.BaseStudyModel) {
	select {
	case bs.calcCh <- model:
	default:
		bs.logger.Warn("study calculation dropped: queue full",
			slog.String("name", bs.name),
		)
	}

	// Reset stale timer on each calculation.
	bs.resetStaleTimer()
}

// ---------------------------------------------------------------------------
// Alert helper
// ---------------------------------------------------------------------------

// TriggerAlert sends an alert value to the alert channel (non-blocking).
func (bs *BaseStudy) TriggerAlert(value decimal.Decimal) {
	select {
	case bs.alertCh <- value:
	default:
		bs.logger.Warn("study alert dropped: channel full",
			slog.String("name", bs.name),
		)
	}
}

// ---------------------------------------------------------------------------
// Consumer loop
// ---------------------------------------------------------------------------

func (bs *BaseStudy) consumeLoop(ctx context.Context) {
	defer close(bs.done)

	// Get the provider channel if subscribed.
	var providerCh <-chan models.Provider
	if bs.bus != nil && bs.providerSubID > 0 {
		// We need to retrieve the channel. Since Subscribe returns it, and we
		// already subscribed in StartAsync, we re-subscribe here for the channel.
		// Actually, the TopicBus.Subscribe approach means we get the channel
		// at subscribe time. Let's restructure: subscribe here instead.
		bs.bus.Providers.Unsubscribe(bs.providerSubID)
		bs.providerSubID, providerCh = bs.bus.Providers.Subscribe(64)
	}

	for {
		select {
		case <-ctx.Done():
			return

		case model, ok := <-bs.calcCh:
			if !ok {
				return
			}
			bs.processCalculation(model)

		case provider, ok := <-providerCh:
			if !ok {
				providerCh = nil
				continue
			}
			bs.handleProviderUpdate(provider)

		case <-bs.staleTimerChan():
			bs.emitStaleIndicator()
		}
	}
}

// staleTimerChan returns the stale timer's channel, or a nil channel if no timer.
func (bs *BaseStudy) staleTimerChan() <-chan time.Time {
	bs.staleMu.Lock()
	defer bs.staleMu.Unlock()
	if bs.staleTimer == nil {
		return nil
	}
	return bs.staleTimer.C
}

// resetStaleTimer resets the stale detection timer.
func (bs *BaseStudy) resetStaleTimer() {
	bs.staleMu.Lock()
	defer bs.staleMu.Unlock()
	if bs.staleTimer != nil {
		// Stop and drain if needed, then reset.
		if !bs.staleTimer.Stop() {
			select {
			case <-bs.staleTimer.C:
			default:
			}
		}
		bs.staleTimer.Reset(bs.staleDuration)
	}
}

func (bs *BaseStudy) emitStaleIndicator() {
	stale := models.BaseStudyModel{
		IsStale:    true,
		Timestamp:  time.Now(),
		Tooltip:    "No data for 30+ seconds",
		ValueColor: "Orange",
	}
	select {
	case bs.outputCh <- stale:
	default:
	}
}

// ---------------------------------------------------------------------------
// Aggregation
// ---------------------------------------------------------------------------

func (bs *BaseStudy) processCalculation(model models.BaseStudyModel) {
	aggDuration := bs.aggLevel.Duration()

	// If aggregation is None (duration == 0) or the item requests skipping.
	if aggDuration == 0 || model.AddItemSkippingAggregation {
		bs.emitOutput(model)
		return
	}

	// Compute the bucket boundary for this item.
	bucketTime := model.Timestamp.Truncate(aggDuration)

	if bs.currentBucket == nil || bucketTime != bs.lastBucketTime {
		// Entering a new bucket - flush the previous bucket first.
		if bs.currentBucket != nil {
			bs.emitOutput(*bs.currentBucket)
		}
		// Start a new bucket with this item.
		bs.lastBucketTime = bucketTime
		itemCopy := model
		bs.currentBucket = &itemCopy
		bs.bucketCount = 1
	} else {
		// Same bucket - aggregate.
		bs.bucketCount++
		if bs.OnDataAggregation != nil {
			bs.OnDataAggregation(bs.currentBucket, model, bs.bucketCount)
		} else {
			// Default aggregation: keep the latest value.
			bs.currentBucket.Value = model.Value
			bs.currentBucket.MarketMidPrice = model.MarketMidPrice
			bs.currentBucket.Timestamp = model.Timestamp
		}
	}
}

func (bs *BaseStudy) emitOutput(model models.BaseStudyModel) {
	select {
	case bs.outputCh <- model:
	default:
		bs.logger.Warn("study output dropped: channel full",
			slog.String("name", bs.name),
		)
	}
}

// FlushAggregation forces the current aggregation bucket to be emitted.
// This is useful for derived studies that need to flush remaining data.
func (bs *BaseStudy) FlushAggregation() {
	if bs.currentBucket != nil {
		bs.emitOutput(*bs.currentBucket)
		bs.currentBucket = nil
		bs.bucketCount = 0
	}
}

// ---------------------------------------------------------------------------
// Provider status monitoring
// ---------------------------------------------------------------------------

func (bs *BaseStudy) handleProviderUpdate(provider models.Provider) {
	if provider.ProviderID != bs.providerID {
		return
	}

	switch provider.Status {
	case enums.SessionDisconnected, enums.SessionDisconnectedFailed:
		errModel := models.BaseStudyModel{
			HasError:   true,
			IsStale:    true,
			Timestamp:  time.Now(),
			Tooltip:    fmt.Sprintf("Provider %s disconnected", provider.ProviderName),
			ValueColor: "Red",
		}
		bs.emitOutput(errModel)

		if provider.Status == enums.SessionDisconnectedFailed {
			bs.SetStatus(enums.PluginStoppedFailed)
		} else {
			bs.SetStatus(enums.PluginStopped)
		}
	}
}

// ---------------------------------------------------------------------------
// Restart logic
// ---------------------------------------------------------------------------

// HandleRestart attempts to restart the study with exponential backoff.
// It is safe to call concurrently; a semaphore ensures only one restart
// runs at a time. After maxRestartAttempts (5) failures, the study status
// is set to StoppedFailed.
func (bs *BaseStudy) HandleRestart(ctx context.Context, reason string, err error) {
	// Acquire restart semaphore (non-blocking if already restarting).
	select {
	case bs.restartSem <- struct{}{}:
		defer func() { <-bs.restartSem }()
	default:
		bs.logger.Warn("study restart already in progress",
			slog.String("name", bs.name),
		)
		return
	}

	bs.restartMu.Lock()
	bs.restartAttempt++
	attempt := bs.restartAttempt
	maxAttempts := bs.maxRestartAttempts
	bs.restartMu.Unlock()

	bs.logger.Warn("study restart requested",
		slog.String("name", bs.name),
		slog.String("reason", reason),
		slog.Int("attempt", attempt),
		slog.Any("error", err),
	)

	if attempt > maxAttempts {
		bs.logger.Error("study restart attempts exhausted",
			slog.String("name", bs.name),
			slog.Int("maxAttempts", maxAttempts),
		)
		bs.SetStatus(enums.PluginStoppedFailed)
		return
	}

	// Exponential backoff: 2^(attempt-1) seconds + jitter.
	backoff := time.Duration(1<<uint(attempt-1)) * time.Second
	jitter := time.Duration(rand.Int63n(int64(time.Second)))
	delay := backoff + jitter

	bs.logger.Info("study restart backoff",
		slog.String("name", bs.name),
		slog.Duration("delay", delay),
	)

	select {
	case <-time.After(delay):
	case <-ctx.Done():
		return
	}

	// Stop then start.
	if stopErr := bs.StopAsync(ctx); stopErr != nil {
		bs.logger.Error("study stop failed during restart",
			slog.String("name", bs.name),
			slog.Any("error", stopErr),
		)
	}

	if startErr := bs.StartAsync(ctx); startErr != nil {
		bs.logger.Error("study start failed during restart",
			slog.String("name", bs.name),
			slog.Any("error", startErr),
		)
		// Recurse for next attempt.
		bs.HandleRestart(ctx, "start failed after restart", startErr)
		return
	}

	// Successful restart - reset attempt counter.
	bs.restartMu.Lock()
	bs.restartAttempt = 0
	bs.restartMu.Unlock()

	bs.logger.Info("study restarted successfully",
		slog.String("name", bs.name),
	)
}

// ---------------------------------------------------------------------------
// Settings helpers
// ---------------------------------------------------------------------------

// SaveToUserSettings persists the given settings object under this study's
// unique ID in the user settings file.
func (bs *BaseStudy) SaveToUserSettings(settings any) error {
	if bs.settings == nil {
		return fmt.Errorf("study: settings manager not configured")
	}
	if err := bs.settings.SetPluginSettings(bs.pluginUniqueID, settings); err != nil {
		return fmt.Errorf("study: saving settings: %w", err)
	}
	return bs.settings.Save()
}

// LoadFromUserSettings loads settings for this study from the user settings
// file into the provided target.
func (bs *BaseStudy) LoadFromUserSettings(target any) error {
	if bs.settings == nil {
		return fmt.Errorf("study: settings manager not configured")
	}
	return bs.settings.GetPluginSettings(bs.pluginUniqueID, target)
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func (bs *BaseStudy) computeUniqueID() string {
	h := sha256.New()
	h.Write([]byte(bs.name))
	h.Write([]byte(bs.author))
	h.Write([]byte(bs.version))
	h.Write([]byte(bs.description))
	return hex.EncodeToString(h.Sum(nil))
}
