// Package ott_ratio implements the Order-to-Trade Ratio (OTR) study plugin.
//
// The OTR is a regulatory metric that measures order book activity relative to
// trade execution. It is calculated as:
//
//	OTR = (addedDelta + deletedDelta + 2*updatedDelta) / max(tradeCount, 1) - 1
//
// Updates are weighted 2x per SEC/FINRA definition because each update
// effectively represents a cancel-and-replace (two actions). A floor of 1 on
// the denominator prevents division by zero when no trades have occurred.
//
// The study operates in L2 mode, deriving order event counts from the
// OrderBook's level change counters (GetCounters/ResetCounters) and counting
// trades from the event bus Trades topic.
//
// Thread safety is achieved via sync/atomic counters for all mutable state
// that is accessed from multiple goroutines.
package ott_ratio

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/shopspring/decimal"
	"github.com/ashark-ai-05/tradefox/internal/config"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
	"github.com/ashark-ai-05/tradefox/internal/study"
)

// OTTRatioStudy computes the Order-to-Trade Ratio from L2 order book delta
// counters and trade events.
type OTTRatioStudy struct {
	*study.BaseStudy

	// Atomic counters for accumulating order events and trades within an
	// aggregation period.
	addedDelta   atomic.Int64
	deletedDelta atomic.Int64
	updatedDelta atomic.Int64
	tradeCount   atomic.Int64

	// lastMarketMidPrice tracks the most recent mid price from the order book.
	lastMarketMidPrice atomic.Value // stores float64

	// Event bus reference (kept separately because BaseStudy.bus is unexported).
	eventBus *eventbus.Bus

	// Event bus subscription IDs (used for cleanup).
	obSubID    uint64
	tradeSubID uint64

	// Lifecycle control for goroutines started by this study.
	cancel context.CancelFunc
	done   chan struct{}

	// logger is kept for study-specific logging.
	logger *slog.Logger
}

// New creates a new OTTRatioStudy with the required dependencies.
func New(bus *eventbus.Bus, settings *config.Manager, logger *slog.Logger) *OTTRatioStudy {
	s := &OTTRatioStudy{
		BaseStudy: study.NewBaseStudy(
			"OTTRatio",
			"1.0.0",
			"Order-to-Trade Ratio",
			"VisualHFT",
			bus,
			settings,
			logger,
		),
		eventBus: bus,
		logger:   logger,
	}
	s.lastMarketMidPrice.Store(float64(0))
	s.SetTileTitle("OTT Ratio")
	s.SetTileToolTip("Order-to-Trade Ratio: measures market activity relative to trade execution")

	// Set the aggregation hook to use "last value" strategy: when multiple
	// calculations land in the same time bucket, keep the latest value.
	s.OnDataAggregation = func(existing *models.BaseStudyModel, newItem models.BaseStudyModel, count int) {
		existing.Value = newItem.Value
		existing.MarketMidPrice = newItem.MarketMidPrice
		existing.Timestamp = newItem.Timestamp
	}

	return s
}

// StartAsync initializes the study: starts the base study pipeline, subscribes
// to OrderBooks and Trades topics on the event bus, and launches goroutines
// to accumulate counters and compute OTR at each aggregation interval.
func (s *OTTRatioStudy) StartAsync(ctx context.Context) error {
	// Start the base study pipeline (consumer loop, stale detection, etc.).
	if err := s.BaseStudy.StartAsync(ctx); err != nil {
		return fmt.Errorf("ott_ratio: starting base study: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.done = make(chan struct{})

	// Subscribe to the event bus topics.
	var obCh <-chan *models.OrderBook
	var tradeCh <-chan models.Trade

	if s.eventBus != nil {
		s.obSubID, obCh = s.eventBus.OrderBooks.Subscribe(256)
		s.tradeSubID, tradeCh = s.eventBus.Trades.Subscribe(256)
	}

	// Launch a single goroutine that handles order book events, trade events,
	// and the aggregation ticker.
	go s.runLoop(ctx, obCh, tradeCh)

	return nil
}

// StopAsync stops the study's goroutines and unsubscribes from the event bus.
func (s *OTTRatioStudy) StopAsync(ctx context.Context) error {
	if s.cancel != nil {
		s.cancel()
	}
	if s.done != nil {
		<-s.done
	}

	// Unsubscribe from event bus topics.
	if s.eventBus != nil {
		if s.obSubID > 0 {
			s.eventBus.OrderBooks.Unsubscribe(s.obSubID)
			s.obSubID = 0
		}
		if s.tradeSubID > 0 {
			s.eventBus.Trades.Unsubscribe(s.tradeSubID)
			s.tradeSubID = 0
		}
	}

	return s.BaseStudy.StopAsync(ctx)
}

// runLoop is the main event processing goroutine. It listens for order book
// updates (to accumulate level change deltas), trade events (to count trades),
// and a periodic ticker (to compute and emit OTR values).
func (s *OTTRatioStudy) runLoop(ctx context.Context, obCh <-chan *models.OrderBook, tradeCh <-chan models.Trade) {
	defer close(s.done)

	// Use a 100ms default tick interval. This provides responsive updates
	// without excessive computation.
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case ob, ok := <-obCh:
			if !ok {
				obCh = nil
				continue
			}
			s.handleOrderBook(ob)

		case _, ok := <-tradeCh:
			if !ok {
				tradeCh = nil
				continue
			}
			s.tradeCount.Add(1)

		case <-ticker.C:
			s.computeAndEmit()
		}
	}
}

// handleOrderBook processes an order book snapshot by reading its level change
// counters and accumulating the deltas into the study's atomic counters.
func (s *OTTRatioStudy) handleOrderBook(ob *models.OrderBook) {
	if ob == nil {
		return
	}

	added, deleted, updated := ob.GetCounters()

	// Accumulate the raw counter values. The formula weights updates 2x
	// at calculation time.
	s.addedDelta.Add(added)
	s.deletedDelta.Add(deleted)
	s.updatedDelta.Add(updated)

	// Track mid price.
	midPrice := ob.MidPrice()
	if midPrice > 0 {
		s.lastMarketMidPrice.Store(midPrice)
	}

	// Reset the order book's counters so the next snapshot provides fresh deltas.
	ob.ResetCounters()
}

// computeAndEmit calculates the OTR from accumulated counters, emits the
// result via AddCalculation, publishes to the Studies bus topic, and resets
// the counters for the next period.
func (s *OTTRatioStudy) computeAndEmit() {
	// Snapshot and reset counters atomically (swap to 0).
	added := s.addedDelta.Swap(0)
	deleted := s.deletedDelta.Swap(0)
	updated := s.updatedDelta.Swap(0)
	trades := s.tradeCount.Swap(0)

	// If there's been no activity at all, skip emitting to avoid noise.
	totalEvents := added + deleted + 2*updated
	if totalEvents == 0 && trades == 0 {
		return
	}

	// OTR formula: (totalEvents) / max(trades, 1) - 1
	denom := trades
	if denom < 1 {
		denom = 1
	}

	otr := decimal.NewFromInt(totalEvents).Div(decimal.NewFromInt(denom)).Sub(decimal.NewFromInt(1))

	midPrice, _ := s.lastMarketMidPrice.Load().(float64)

	item := models.BaseStudyModel{
		Value:          otr,
		MarketMidPrice: midPrice,
		Timestamp:      time.Now(),
	}

	// Feed into the base study aggregation pipeline.
	s.AddCalculation(item)

	// Also publish to the Studies bus topic for other consumers.
	if s.eventBus != nil {
		s.eventBus.Studies.Publish(item)
	}
}

// ResetCounters resets all atomic counters to zero. This is useful for testing
// and is called internally at each aggregation tick.
func (s *OTTRatioStudy) ResetCounters() {
	s.addedDelta.Store(0)
	s.deletedDelta.Store(0)
	s.updatedDelta.Store(0)
	s.tradeCount.Store(0)
}

// GetCounters returns the current values of all atomic counters. This is
// primarily useful for testing and debugging.
func (s *OTTRatioStudy) GetCounters() (added, deleted, updated, trades int64) {
	return s.addedDelta.Load(), s.deletedDelta.Load(), s.updatedDelta.Load(), s.tradeCount.Load()
}
