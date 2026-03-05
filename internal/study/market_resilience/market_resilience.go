package marketresilience

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/shopspring/decimal"
	"github.com/ashark-ai-05/tradefox/internal/config"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
	"github.com/ashark-ai-05/tradefox/internal/study"
)

// MarketResilienceStudy measures how quickly a market recovers after large
// trade shocks. It combines trade shock detection, spread widening, LOB depth
// depletion, and recovery tracking to produce a score in [0, 1].
type MarketResilienceStudy struct {
	*study.BaseStudy

	calculator *MarketResilienceCalculator
	eventBus   *eventbus.Bus

	obSubID    uint64
	tradeSubID uint64

	cancel context.CancelFunc
	done   chan struct{}

	logger *slog.Logger
}

// New creates a new MarketResilienceStudy with the required dependencies.
func New(bus *eventbus.Bus, settings *config.Manager, logger *slog.Logger) *MarketResilienceStudy {
	s := &MarketResilienceStudy{
		BaseStudy: study.NewBaseStudy(
			"MarketResilience",
			"1.0.0",
			"Market Resilience: measures recovery speed after trade shocks",
			"VisualHFT",
			bus,
			settings,
			logger,
		),
		calculator: NewCalculator(5000), // 5-second default shock timeout
		eventBus:   bus,
		logger:     logger,
	}
	s.SetTileTitle("Market Resilience")
	s.SetTileToolTip("Market Resilience: speed of recovery after large trade shocks (0=slow, 1=fast)")

	// Wire calculator output to the study pipeline.
	s.calculator.OnScoreCalculated = func(score decimal.Decimal, midPrice float64) {
		item := models.BaseStudyModel{
			Value:          score,
			MarketMidPrice: midPrice,
			Timestamp:      time.Now(),
		}
		s.AddCalculation(item)
		if s.eventBus != nil {
			s.eventBus.Studies.Publish(item)
		}
	}

	// Keep-latest aggregation strategy.
	s.OnDataAggregation = func(existing *models.BaseStudyModel, newItem models.BaseStudyModel, count int) {
		existing.Value = newItem.Value
		existing.MarketMidPrice = newItem.MarketMidPrice
		existing.Timestamp = newItem.Timestamp
	}

	return s
}

// StartAsync starts the study, subscribes to event bus topics, and launches
// the processing goroutine.
func (s *MarketResilienceStudy) StartAsync(ctx context.Context) error {
	if err := s.BaseStudy.StartAsync(ctx); err != nil {
		return fmt.Errorf("market_resilience: starting base study: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.done = make(chan struct{})

	var obCh <-chan *models.OrderBook
	var tradeCh <-chan models.Trade

	if s.eventBus != nil {
		s.obSubID, obCh = s.eventBus.OrderBooks.Subscribe(256)
		s.tradeSubID, tradeCh = s.eventBus.Trades.Subscribe(256)
	}

	go s.runLoop(ctx, obCh, tradeCh)
	return nil
}

// StopAsync stops the study and unsubscribes from the event bus.
func (s *MarketResilienceStudy) StopAsync(ctx context.Context) error {
	if s.cancel != nil {
		s.cancel()
	}
	if s.done != nil {
		<-s.done
	}

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

func (s *MarketResilienceStudy) runLoop(ctx context.Context, obCh <-chan *models.OrderBook, tradeCh <-chan models.Trade) {
	defer close(s.done)

	for {
		select {
		case <-ctx.Done():
			return
		case ob, ok := <-obCh:
			if !ok {
				obCh = nil
				continue
			}
			if ob != nil {
				snap := SnapshotFromOrderBook(ob)
				s.calculator.OnOrderBookUpdate(snap)
			}
		case trade, ok := <-tradeCh:
			if !ok {
				tradeCh = nil
				continue
			}
			s.calculator.OnTrade(trade)
		}
	}
}

// Calculator returns the internal calculator for testing purposes.
func (s *MarketResilienceStudy) Calculator() *MarketResilienceCalculator {
	return s.calculator
}
