package liquidation

import (
	"log/slog"
	"os"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestGenerateHeatmap_ProducesValidBands(t *testing.T) {
	tracker := NewTracker()
	engine := NewHeatmapEngine(tracker, testLogger())

	// Add some position data
	now := time.Now().UnixMilli()
	tracker.ProcessOIChange("BTCUSDT", 95000, 1000, now)
	tracker.ProcessOIChange("BTCUSDT", 100000, 2000, now)
	tracker.ProcessOIChange("BTCUSDT", 105000, 1500, now)

	data := engine.GenerateHeatmap("BTCUSDT", 100000, 5, 100)

	if data.Symbol != "BTCUSDT" {
		t.Errorf("expected symbol BTCUSDT, got %s", data.Symbol)
	}
	if data.CurrentPrice != 100000 {
		t.Errorf("expected price 100000, got %v", data.CurrentPrice)
	}
	if len(data.Bands) != 100 {
		t.Fatalf("expected 100 bands, got %d", len(data.Bands))
	}

	// Bands should be ordered by price
	for i := 1; i < len(data.Bands); i++ {
		if data.Bands[i].PriceMin < data.Bands[i-1].PriceMin {
			t.Errorf("bands not ordered: band[%d].PriceMin=%v < band[%d].PriceMin=%v",
				i, data.Bands[i].PriceMin, i-1, data.Bands[i-1].PriceMin)
		}
	}

	// Should have some volume
	var totalVol float64
	for _, b := range data.Bands {
		totalVol += b.LongLiqVolume + b.ShortLiqVolume
	}
	if totalVol == 0 {
		t.Error("expected some liquidation volume in bands")
	}
}

func TestGenerateHeatmap_EmptyPositions(t *testing.T) {
	tracker := NewTracker()
	engine := NewHeatmapEngine(tracker, testLogger())

	data := engine.GenerateHeatmap("BTCUSDT", 100000, 5, 100)

	if len(data.Bands) != 100 {
		t.Fatalf("expected 100 bands even with no data, got %d", len(data.Bands))
	}

	// All bands should have zero volume
	for _, b := range data.Bands {
		if b.LongLiqVolume != 0 || b.ShortLiqVolume != 0 {
			t.Error("expected zero volume for empty positions")
		}
	}
}

func TestHeatmapStats_Asymmetry(t *testing.T) {
	tracker := NewTracker()
	engine := NewHeatmapEngine(tracker, testLogger())

	now := time.Now().UnixMilli()
	// Add positions above current price (will generate short liquidations above)
	tracker.ProcessOIChange("BTCUSDT", 102000, 5000, now)
	// Add smaller positions below
	tracker.ProcessOIChange("BTCUSDT", 98000, 1000, now)

	data := engine.GenerateHeatmap("BTCUSDT", 100000, 10, 200)

	// Stats should reflect the asymmetry
	if data.Stats.TotalAbove == 0 && data.Stats.TotalBelow == 0 {
		t.Error("expected non-zero totals")
	}
}

func TestHeatmapStats_MagnetDirection(t *testing.T) {
	bands := []HeatmapBand{
		{PriceMin: 95000, PriceMax: 96000, LongLiqVolume: 100, ShortLiqVolume: 0},
		{PriceMin: 104000, PriceMax: 105000, LongLiqVolume: 0, ShortLiqVolume: 500},
	}

	stats := computeStats(bands, 100000)

	if stats.TotalAbove <= stats.TotalBelow {
		t.Error("expected more volume above price")
	}
	if stats.MagnetDirection != "Up" {
		t.Errorf("expected magnet direction Up, got %s", stats.MagnetDirection)
	}
}

func TestHeatmapStats_NeutralMagnet(t *testing.T) {
	bands := []HeatmapBand{
		{PriceMin: 95000, PriceMax: 96000, LongLiqVolume: 100, ShortLiqVolume: 0},
		{PriceMin: 104000, PriceMax: 105000, LongLiqVolume: 0, ShortLiqVolume: 100},
	}

	stats := computeStats(bands, 100000)

	if stats.MagnetDirection != "Neutral" {
		t.Errorf("expected Neutral magnet for equal volume, got %s", stats.MagnetDirection)
	}
}

func TestUpdatePrice_And_Latest(t *testing.T) {
	tracker := NewTracker()
	engine := NewHeatmapEngine(tracker, testLogger())

	engine.UpdatePrice("BTCUSDT", 100000)

	// Latest should be nil before first generation
	if got := engine.Latest("BTCUSDT"); got != nil {
		t.Error("expected nil before first generation")
	}
}

func TestLiquidationFeed_RecentEvents(t *testing.T) {
	feed := NewLiquidationFeed()

	for i := 0; i < 10; i++ {
		feed.Add(LiquidationEvent{
			Symbol:   "BTCUSDT",
			Side:     "Sell",
			Price:    float64(95000 + i),
			Quantity: 1.0,
			Notional: float64(95000 + i),
			Time:     int64(1000 + i),
		})
	}

	events := feed.RecentEvents("BTCUSDT", 5)
	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}

	// Most recent first
	if events[0].Time != 1009 {
		t.Errorf("expected most recent event first (time=1009), got %d", events[0].Time)
	}
}

func TestLiquidationFeed_Stats(t *testing.T) {
	feed := NewLiquidationFeed()

	now := time.Now().UnixMilli()
	feed.Add(LiquidationEvent{Symbol: "BTCUSDT", Side: "Sell", Notional: 50000, Time: now})
	feed.Add(LiquidationEvent{Symbol: "BTCUSDT", Side: "Buy", Notional: 30000, Time: now})
	feed.Add(LiquidationEvent{Symbol: "BTCUSDT", Side: "Sell", Notional: 100000, Time: now})

	stats := feed.Stats("BTCUSDT", time.Hour)

	if stats.Count != 3 {
		t.Errorf("expected count 3, got %d", stats.Count)
	}
	if stats.LongsLiquidated != 150000 {
		t.Errorf("expected longs liquidated 150000, got %v", stats.LongsLiquidated)
	}
	if stats.ShortsLiquidated != 30000 {
		t.Errorf("expected shorts liquidated 30000, got %v", stats.ShortsLiquidated)
	}
	if stats.LargestSingle.Notional != 100000 {
		t.Errorf("expected largest single 100000, got %v", stats.LargestSingle.Notional)
	}
}
