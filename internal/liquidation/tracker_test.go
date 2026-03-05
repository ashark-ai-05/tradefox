package liquidation

import (
	"testing"
	"time"
)

func TestProcessOIChange_Accumulates(t *testing.T) {
	tracker := NewTracker()

	tracker.ProcessOIChange("BTCUSDT", 100000, 500, time.Now().UnixMilli())
	tracker.ProcessOIChange("BTCUSDT", 101000, 300, time.Now().UnixMilli())

	positions := tracker.GetPositionMap("BTCUSDT")

	// Each OI change produces 2 estimates (long + short), so 2 changes = 4 estimates
	if len(positions) != 4 {
		t.Fatalf("expected 4 position estimates, got %d", len(positions))
	}

	// Verify sides are alternating long/short
	if positions[0].Side != "long" || positions[1].Side != "short" {
		t.Errorf("expected first pair to be long/short, got %s/%s", positions[0].Side, positions[1].Side)
	}

	// Each side gets half the volume
	if positions[0].Volume != 250 {
		t.Errorf("expected volume 250, got %v", positions[0].Volume)
	}
}

func TestProcessOIChange_NegativeDelta(t *testing.T) {
	tracker := NewTracker()

	tracker.ProcessOIChange("BTCUSDT", 100000, -500, time.Now().UnixMilli())

	positions := tracker.GetPositionMap("BTCUSDT")
	if len(positions) != 0 {
		t.Errorf("expected 0 positions for negative OI delta, got %d", len(positions))
	}
}

func TestProcessOIChange_ZeroDelta(t *testing.T) {
	tracker := NewTracker()

	tracker.ProcessOIChange("BTCUSDT", 100000, 0, time.Now().UnixMilli())

	positions := tracker.GetPositionMap("BTCUSDT")
	if len(positions) != 0 {
		t.Errorf("expected 0 positions for zero OI delta, got %d", len(positions))
	}
}

func TestDecayOldPositions(t *testing.T) {
	tracker := NewTracker()

	oldTime := time.Now().Add(-100 * time.Hour).UnixMilli()
	recentTime := time.Now().UnixMilli()

	tracker.ProcessOIChange("BTCUSDT", 100000, 500, oldTime)
	tracker.ProcessOIChange("BTCUSDT", 101000, 300, recentTime)

	// Before decay: 4 positions (2 old + 2 recent)
	if count := tracker.PositionCount("BTCUSDT"); count != 4 {
		t.Fatalf("expected 4 before decay, got %d", count)
	}

	tracker.DecayOldPositions(72 * time.Hour)

	// After decay: only 2 recent positions should remain
	positions := tracker.GetPositionMap("BTCUSDT")
	if len(positions) != 2 {
		t.Fatalf("expected 2 after decay, got %d", len(positions))
	}

	for _, p := range positions {
		if p.EntryPrice != 101000 {
			t.Errorf("expected only recent positions (101000), got %v", p.EntryPrice)
		}
	}
}

func TestGetPositionMap_EmptySymbol(t *testing.T) {
	tracker := NewTracker()

	positions := tracker.GetPositionMap("NONEXISTENT")
	if positions != nil {
		t.Errorf("expected nil for non-existent symbol, got %v", positions)
	}
}

func TestGetPositionMap_ReturnsCopy(t *testing.T) {
	tracker := NewTracker()

	tracker.ProcessOIChange("BTCUSDT", 100000, 500, time.Now().UnixMilli())

	pos1 := tracker.GetPositionMap("BTCUSDT")
	pos2 := tracker.GetPositionMap("BTCUSDT")

	// Modifying the returned slice should not affect the tracker
	pos1[0].Volume = 999999
	if pos2[0].Volume == 999999 {
		t.Error("GetPositionMap should return a copy, not a reference")
	}
}

func TestMultipleSymbols(t *testing.T) {
	tracker := NewTracker()

	tracker.ProcessOIChange("BTCUSDT", 100000, 500, time.Now().UnixMilli())
	tracker.ProcessOIChange("ETHUSDT", 3000, 200, time.Now().UnixMilli())

	btc := tracker.GetPositionMap("BTCUSDT")
	eth := tracker.GetPositionMap("ETHUSDT")

	if len(btc) != 2 {
		t.Errorf("expected 2 BTC positions, got %d", len(btc))
	}
	if len(eth) != 2 {
		t.Errorf("expected 2 ETH positions, got %d", len(eth))
	}
}
