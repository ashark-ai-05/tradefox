package ott_ratio

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

func testBus() *eventbus.Bus {
	return eventbus.NewBus(slog.Default())
}

func TestOTTRatio_Formula(t *testing.T) {
	// OTR = (added + deleted + 2*updated) / max(trades, 1) - 1
	// 50 added + 30 deleted + 2*10 updated = 100 events
	// 10 trades → OTR = (100/10) - 1 = 9
	bus := testBus()
	defer bus.Close()

	s := New(bus, nil, slog.Default())
	s.addedDelta.Store(50)
	s.deletedDelta.Store(30)
	s.updatedDelta.Store(10)
	s.tradeCount.Store(10)
	s.lastMarketMidPrice.Store(float64(100.0))

	// Capture the emitted calculation.
	ctx := context.Background()
	if err := s.BaseStudy.StartAsync(ctx); err != nil {
		t.Fatal(err)
	}

	calcCh := s.OnCalculated()

	s.computeAndEmit()

	select {
	case item := <-calcCh:
		expected := decimal.NewFromInt(9)
		if !item.Value.Equal(expected) {
			t.Errorf("expected OTR=9, got %s", item.Value.String())
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for OTR calculation")
	}

	_ = s.BaseStudy.StopAsync(ctx)
}

func TestOTTRatio_ZeroTrades(t *testing.T) {
	// No trades → denom = max(0, 1) = 1
	// 10 added + 0 deleted + 0 updated = 10 events
	// OTR = (10/1) - 1 = 9
	bus := testBus()
	defer bus.Close()

	s := New(bus, nil, slog.Default())
	s.addedDelta.Store(10)
	s.deletedDelta.Store(0)
	s.updatedDelta.Store(0)
	s.tradeCount.Store(0)

	ctx := context.Background()
	if err := s.BaseStudy.StartAsync(ctx); err != nil {
		t.Fatal(err)
	}

	calcCh := s.OnCalculated()
	s.computeAndEmit()

	select {
	case item := <-calcCh:
		expected := decimal.NewFromInt(9)
		if !item.Value.Equal(expected) {
			t.Errorf("expected OTR=9, got %s", item.Value.String())
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for OTR calculation")
	}

	_ = s.BaseStudy.StopAsync(ctx)
}

func TestOTTRatio_NoActivity(t *testing.T) {
	// No events at all → should not emit.
	bus := testBus()
	defer bus.Close()

	s := New(bus, nil, slog.Default())

	ctx := context.Background()
	if err := s.BaseStudy.StartAsync(ctx); err != nil {
		t.Fatal(err)
	}

	calcCh := s.OnCalculated()
	s.computeAndEmit()

	select {
	case <-calcCh:
		t.Error("should not emit when there is no activity")
	case <-time.After(100 * time.Millisecond):
		// Expected: no emission.
	}

	_ = s.BaseStudy.StopAsync(ctx)
}

func TestOTTRatio_CounterReset(t *testing.T) {
	bus := testBus()
	defer bus.Close()

	s := New(bus, nil, slog.Default())
	s.addedDelta.Store(50)
	s.deletedDelta.Store(30)
	s.updatedDelta.Store(10)
	s.tradeCount.Store(5)

	ctx := context.Background()
	if err := s.BaseStudy.StartAsync(ctx); err != nil {
		t.Fatal(err)
	}

	s.computeAndEmit()
	// Drain the output.
	select {
	case <-s.OnCalculated():
	case <-time.After(time.Second):
	}

	// Counters should be reset after compute.
	added, deleted, updated, trades := s.GetCounters()
	if added != 0 || deleted != 0 || updated != 0 || trades != 0 {
		t.Errorf("expected all counters to be 0 after compute, got a=%d d=%d u=%d t=%d",
			added, deleted, updated, trades)
	}

	_ = s.BaseStudy.StopAsync(ctx)
}

func TestOTTRatio_Metadata(t *testing.T) {
	bus := testBus()
	defer bus.Close()

	s := New(bus, nil, slog.Default())
	if s.Name() != "OTTRatio" {
		t.Errorf("expected name OTTRatio, got %s", s.Name())
	}
	if s.Version() != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", s.Version())
	}
	if s.TileTitle() != "OTT Ratio" {
		t.Errorf("expected tile title 'OTT Ratio', got %s", s.TileTitle())
	}
}

func TestOTTRatio_EventBusIntegration(t *testing.T) {
	bus := testBus()
	defer bus.Close()

	s := New(bus, nil, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer func() { _ = s.StopAsync(context.Background()) }()

	// Publish some order books with level counters.
	ob := models.NewOrderBook("BTCUSD", 2, 5)
	bidPrice := 30000.0
	bidSize := 1.0
	askPrice := 30001.0
	askSize := 1.0
	isBid := true
	isAsk := false
	ob.AddOrUpdateLevel(models.DeltaBookItem{Price: &bidPrice, Size: &bidSize, IsBid: &isBid})
	ob.AddOrUpdateLevel(models.DeltaBookItem{Price: &askPrice, Size: &askSize, IsBid: &isAsk})

	// Publish order book multiple times to generate adds/updates.
	for i := 0; i < 5; i++ {
		bus.OrderBooks.Publish(ob)
	}

	// Publish some trades.
	for i := 0; i < 3; i++ {
		bus.Trades.Publish(models.Trade{
			Symbol: "BTCUSD",
			Price:  decimal.NewFromFloat(30000.5),
			Size:   decimal.NewFromFloat(0.1),
		})
	}

	// Wait for events to be processed and ticker to fire.
	time.Sleep(300 * time.Millisecond)

	// Verify that counters accumulated (exact values depend on timing).
	// The study will have processed some of the events.
	// This is mainly a smoke test for the integration.
}
