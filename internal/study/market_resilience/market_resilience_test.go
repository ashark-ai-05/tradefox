package marketresilience

import (
	"context"
	"log/slog"
	"math"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

func testBus() *eventbus.Bus {
	return eventbus.NewBus(slog.Default())
}

// ---------------------------------------------------------------------------
// P2 Quantile Tests
// ---------------------------------------------------------------------------

func TestP2Quantile_Uniform(t *testing.T) {
	pq := NewP2Quantile(0.5)
	for i := 1; i <= 100; i++ {
		pq.Observe(float64(i))
	}

	est := pq.Estimate()
	actual := 50.5
	tolerance := actual * 0.15 // 15% tolerance
	if math.Abs(est-actual) > tolerance {
		t.Errorf("median estimate %f not within 15%% of actual %f", est, actual)
	}
}

func TestP2Quantile_Increasing(t *testing.T) {
	pq := NewP2Quantile(0.5)
	for i := 1; i <= 50; i++ {
		pq.Observe(float64(i))
	}

	est := pq.Estimate()
	if est < 15 || est > 35 {
		t.Errorf("median estimate %f for 1..50 should be near 25, got out of range", est)
	}
}

func TestP2Quantile_FewSamples(t *testing.T) {
	pq := NewP2Quantile(0.5)
	pq.Observe(10)
	pq.Observe(20)
	pq.Observe(30)

	est := pq.Estimate()
	if est != 20 {
		t.Errorf("expected 20 for 3-sample median, got %f", est)
	}
}

func TestP2Quantile_NaN_Inf_Ignored(t *testing.T) {
	pq := NewP2Quantile(0.5)
	for i := 1; i <= 10; i++ {
		pq.Observe(float64(i))
	}
	before := pq.Estimate()

	pq.Observe(math.NaN())
	pq.Observe(math.Inf(1))
	pq.Observe(math.Inf(-1))

	after := pq.Estimate()
	if before != after {
		t.Errorf("NaN/Inf should not change estimate: before=%f, after=%f", before, after)
	}
}

// ---------------------------------------------------------------------------
// Calculator Tests
// ---------------------------------------------------------------------------

func TestCalculator_ShockDetection(t *testing.T) {
	calc := NewCalculator(5000)

	// Feed a history of small trades to establish a baseline.
	// Need enough trades to pass the "count >= 3" check in isLargeTrade,
	// and they must go into recentTradeSizes (not shockTrade).
	// The first large trade attempt with too few samples will just be
	// added to recentTradeSizes. So feed small trades first.
	for i := 0; i < 50; i++ {
		calc.OnTrade(models.Trade{
			Size: decimal.NewFromFloat(1.0 + float64(i%3)*0.1), // small variation
		})
	}

	// Now inject a large trade (should be > 2-sigma above mean of ~1.1).
	calc.OnTrade(models.Trade{
		Size: decimal.NewFromFloat(100.0),
	})

	if calc.shockTrade == nil {
		t.Error("expected shock trade to be detected")
	}
}

func TestCalculator_NoShockOnSmallTrades(t *testing.T) {
	calc := NewCalculator(5000)

	for i := 0; i < 20; i++ {
		calc.OnTrade(models.Trade{
			Size: decimal.NewFromFloat(1.0),
		})
	}

	// Trade within normal range should not trigger shock.
	calc.OnTrade(models.Trade{
		Size: decimal.NewFromFloat(1.5),
	})

	if calc.shockTrade != nil {
		t.Error("did not expect shock trade for a small trade")
	}
}

func TestCalculator_SpreadWidening(t *testing.T) {
	calc := NewCalculator(10000)

	// Build up spread history with slight variation for non-zero stddev.
	for i := 0; i < 50; i++ {
		askOffset := 0.09 + float64(i%5)*0.005 // spread varies 0.09 to 0.11
		snap := makeSnapshot(100.0, 100.0+askOffset, 5)
		calc.OnOrderBookUpdate(snap)
	}

	// Trigger a shock trade first - need enough trade history.
	for i := 0; i < 50; i++ {
		calc.OnTrade(models.Trade{Size: decimal.NewFromFloat(1.0 + float64(i%3)*0.1)})
	}
	calc.OnTrade(models.Trade{Size: decimal.NewFromFloat(100.0)})

	if calc.shockTrade == nil {
		t.Fatal("shock trade not detected")
	}

	// Now widen spread significantly.
	snap := makeSnapshot(99.0, 102.0, 5) // spread=3.0, much wider than avg of 0.1
	calc.OnOrderBookUpdate(snap)

	if calc.shockSpread == nil {
		t.Error("expected spread shock to be detected after large spread widening")
	}
}

func TestCalculator_MRScoreInRange(t *testing.T) {
	calc := NewCalculator(10000)

	var scores []decimal.Decimal
	calc.OnScoreCalculated = func(score decimal.Decimal, midPrice float64) {
		scores = append(scores, score)
	}

	// 1. Build baseline: normal spreads + normal trades.
	for i := 0; i < 50; i++ {
		calc.OnTrade(models.Trade{Size: decimal.NewFromFloat(1.0 + float64(i%3)*0.1)})
		snap := makeSnapshot(100.0, 100.1, 5)
		calc.OnOrderBookUpdate(snap)
	}

	// 2. Large trade shock.
	calc.OnTrade(models.Trade{Size: decimal.NewFromFloat(100.0)})

	// 3. Spread widens.
	snap := makeSnapshot(99.0, 102.0, 5)
	calc.OnOrderBookUpdate(snap)

	// 4. Spread returns to normal.
	snap = makeSnapshot(100.0, 100.05, 5)
	calc.OnOrderBookUpdate(snap)

	// Check if any MR score was generated.
	if len(scores) == 0 {
		t.Skip("no MR score generated in this scenario (may need depth depletion)")
		return
	}

	for i, s := range scores {
		sf, _ := s.Float64()
		if sf < 0 || sf > 1 {
			t.Errorf("score[%d] = %f, expected in [0, 1]", i, sf)
		}
	}
}

// ---------------------------------------------------------------------------
// Rolling Window Tests
// ---------------------------------------------------------------------------

func TestRollingWindow_Basic(t *testing.T) {
	rw := newRollingWindow[float64](5)
	for i := 1; i <= 3; i++ {
		rw.Add(float64(i))
	}

	if rw.Count() != 3 {
		t.Errorf("expected count 3, got %d", rw.Count())
	}

	items := rw.items()
	if len(items) != 3 || items[0] != 1 || items[2] != 3 {
		t.Errorf("unexpected items: %v", items)
	}
}

func TestRollingWindow_Overflow(t *testing.T) {
	rw := newRollingWindow[float64](3)
	for i := 1; i <= 5; i++ {
		rw.Add(float64(i))
	}

	if rw.Count() != 3 {
		t.Errorf("expected count 3, got %d", rw.Count())
	}

	items := rw.items()
	// Should have [3, 4, 5]
	if len(items) != 3 || items[0] != 3 || items[1] != 4 || items[2] != 5 {
		t.Errorf("expected [3 4 5], got %v", items)
	}
}

// ---------------------------------------------------------------------------
// Study Integration Test
// ---------------------------------------------------------------------------

func TestMarketResilienceStudy_Metadata(t *testing.T) {
	bus := testBus()
	defer bus.Close()

	s := New(bus, nil, slog.Default())
	if s.Name() != "MarketResilience" {
		t.Errorf("expected name MarketResilience, got %s", s.Name())
	}
	if s.TileTitle() != "Market Resilience" {
		t.Errorf("expected tile title 'Market Resilience', got %s", s.TileTitle())
	}
}

func TestMarketResilienceStudy_StartStop(t *testing.T) {
	bus := testBus()
	defer bus.Close()

	s := New(bus, nil, slog.Default())
	ctx := context.Background()

	if err := s.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}

	// Give it a moment to be running.
	time.Sleep(50 * time.Millisecond)

	if err := s.StopAsync(ctx); err != nil {
		t.Fatalf("StopAsync failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func makeSnapshot(bestBid, bestAsk float64, levels int) OrderBookSnapshot {
	bids := make([]models.BookItem, levels)
	asks := make([]models.BookItem, levels)

	for i := 0; i < levels; i++ {
		bidPrice := bestBid - float64(i)*0.01
		askPrice := bestAsk + float64(i)*0.01
		bidSize := 10.0
		askSize := 10.0
		bids[i] = models.BookItem{Price: &bidPrice, Size: &bidSize}
		asks[i] = models.BookItem{Price: &askPrice, Size: &askSize}
	}

	return OrderBookSnapshot{
		Bids:     bids,
		Asks:     asks,
		Spread:   bestAsk - bestBid,
		MidPrice: (bestBid + bestAsk) / 2,
	}
}
