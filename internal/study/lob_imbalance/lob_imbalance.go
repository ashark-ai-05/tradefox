// Package lobimbalance implements the Limit Order Book Imbalance study plugin.
//
// The LOB Imbalance measures the disparity between aggregate bid and ask
// volume in the top N levels of the order book:
//
//	Imbalance = (totalBidSize - totalAskSize) / (totalBidSize + totalAskSize)
//
// The result ranges from -1 (all volume on the ask side) to +1 (all volume
// on the bid side). A value near 0 indicates balanced supply and demand.
package lobimbalance

import (
	"context"
	"log/slog"
	"time"

	"github.com/shopspring/decimal"
	"github.com/ashark-ai-05/tradefox/internal/config"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
	"github.com/ashark-ai-05/tradefox/internal/study"
)

// DefaultBookDepth is the number of top price levels to consider on each
// side of the order book when calculating the imbalance ratio.
const DefaultBookDepth = 5

// LOBImbalanceStudy calculates the Limit Order Book Imbalance metric by
// subscribing to order book snapshots and computing the ratio of bid to ask
// volume across the top N price levels.
type LOBImbalanceStudy struct {
	*study.BaseStudy
	bus       *eventbus.Bus
	bookDepth int
}

// New creates a new LOBImbalanceStudy with default settings.
func New(bus *eventbus.Bus, settings *config.Manager, logger *slog.Logger) *LOBImbalanceStudy {
	s := &LOBImbalanceStudy{
		BaseStudy: study.NewBaseStudy(
			"LOBImbalance",
			"1.0.0",
			"Limit Order Book Imbalance",
			"VisualHFT",
			bus,
			settings,
			logger,
		),
		bus:       bus,
		bookDepth: DefaultBookDepth,
	}
	s.SetTileTitle("LOB Imbalance")
	s.SetTileToolTip("Limit Order Book Imbalance: (bidVol - askVol) / (bidVol + askVol)")

	// Aggregation hook: keep the latest value (same as C# onDataAggregation).
	s.OnDataAggregation = func(existing *models.BaseStudyModel, newItem models.BaseStudyModel, _ int) {
		existing.Value = newItem.Value
		existing.MarketMidPrice = newItem.MarketMidPrice
		existing.Timestamp = newItem.Timestamp
	}

	return s
}

// SetBookDepth overrides the default number of price levels used for the
// imbalance calculation. Must be called before StartAsync.
func (s *LOBImbalanceStudy) SetBookDepth(depth int) {
	if depth > 0 {
		s.bookDepth = depth
	}
}

// BookDepth returns the currently configured book depth.
func (s *LOBImbalanceStudy) BookDepth() int {
	return s.bookDepth
}

// StartAsync initialises the base study pipeline, subscribes to the
// OrderBooks topic on the event bus, and starts a goroutine that processes
// incoming order book snapshots.
func (s *LOBImbalanceStudy) StartAsync(ctx context.Context) error {
	if err := s.BaseStudy.StartAsync(ctx); err != nil {
		return err
	}

	subID, obCh := s.bus.OrderBooks.Subscribe(256)

	go s.processLoop(ctx, subID, obCh)

	return nil
}

// processLoop reads order book snapshots from the event bus, computes the
// imbalance ratio, and feeds results into the base study pipeline via
// AddCalculation. It also publishes each result to the Studies topic on
// the bus so other components can consume study outputs.
func (s *LOBImbalanceStudy) processLoop(ctx context.Context, subID uint64, obCh <-chan *models.OrderBook) {
	defer s.bus.OrderBooks.Unsubscribe(subID)

	for {
		select {
		case <-ctx.Done():
			return
		case ob, ok := <-obCh:
			if !ok {
				return
			}
			s.handleOrderBook(ob)
		}
	}
}

// handleOrderBook computes the imbalance from the given order book snapshot
// and feeds the result into the study pipeline.
func (s *LOBImbalanceStudy) handleOrderBook(ob *models.OrderBook) {
	if ob == nil {
		return
	}

	bids := ob.Bids()
	asks := ob.Asks()

	totalBid := sumTopN(bids, s.bookDepth)
	totalAsk := sumTopN(asks, s.bookDepth)

	total := totalBid + totalAsk
	if total == 0 {
		return
	}

	imbalance := (totalBid - totalAsk) / total

	model := models.BaseStudyModel{
		Value:          decimal.NewFromFloat(imbalance),
		MarketMidPrice: ob.MidPrice(),
		Timestamp:      time.Now(),
	}

	s.AddCalculation(model)

	// Also publish to the Studies topic so downstream consumers (e.g. UI,
	// alerting) can react to this study's output.
	if s.bus != nil {
		s.bus.Studies.Publish(model)
	}
}

// sumTopN returns the sum of sizes across the first n levels. If the slice
// has fewer than n elements, all available levels are summed.
func sumTopN(levels []models.BookItem, n int) float64 {
	if n > len(levels) {
		n = len(levels)
	}
	var total float64
	for i := 0; i < n; i++ {
		if levels[i].Size != nil {
			total += *levels[i].Size
		}
	}
	return total
}
