package backtest

import (
	"math"
	"testing"
)

func TestPositionManager_OpenAndStop(t *testing.T) {
	pm := NewPositionManager(DefaultPositionConfig(), 10000)
	pm.OpenPosition("SOLUSDT", 1, 100.0, 1.0, 2.0, 1000)

	if !pm.HasPosition("SOLUSDT") {
		t.Fatal("expected open position")
	}

	// Price drops below stop (100 - 1.5*2 = 97)
	ct := pm.Update("SOLUSDT", 96.0, 2000, 0.1)
	if ct == nil {
		t.Fatal("expected closed trade")
	}
	if ct.ExitReason != "stop" {
		t.Errorf("expected stop, got %s", ct.ExitReason)
	}
	if ct.PnLPct >= 0 {
		t.Error("expected negative P&L")
	}
}

func TestPositionManager_Target(t *testing.T) {
	// Use a config where phase 3 trigger is above 1.0 so target fires first.
	cfg := DefaultPositionConfig()
	cfg.Phase3Trigger = 1.5 // won't trigger at target
	pm := NewPositionManager(cfg, 10000)
	pm.OpenPosition("SOLUSDT", 1, 100.0, 1.0, 2.0, 1000)

	// Price rises to target (100 + 3*2 = 106)
	ct := pm.Update("SOLUSDT", 107.0, 2000, 0.1)
	if ct == nil {
		t.Fatal("expected closed trade")
	}
	if ct.ExitReason != "target" {
		t.Errorf("expected target, got %s", ct.ExitReason)
	}
	if ct.PnLPct <= 0 {
		t.Error("expected positive P&L")
	}
}

func TestPositionManager_TargetDefaultBecomesTrailing(t *testing.T) {
	// With default config, reaching target triggers phase 3 trailing stop.
	pm := NewPositionManager(DefaultPositionConfig(), 10000)
	pm.OpenPosition("SOLUSDT", 1, 100.0, 1.0, 2.0, 1000)

	// Price reaches target zone; phase 3 kicks in, so no immediate exit
	ct := pm.Update("SOLUSDT", 107.0, 2000, 0)
	if ct != nil {
		t.Error("with default config, reaching target should trigger trailing, not immediate exit")
	}
	pos := pm.positions["SOLUSDT"]
	if pos.Phase != 3 {
		t.Errorf("expected phase 3, got %d", pos.Phase)
	}

	// Now price drops to trailing stop: highWater(107) - 0.08*2 = 106.84
	ct = pm.Update("SOLUSDT", 106.5, 3000, 0.1)
	if ct == nil {
		t.Fatal("expected trailing stop exit")
	}
	if ct.ExitReason != "trailing" {
		t.Errorf("expected trailing, got %s", ct.ExitReason)
	}
}

func TestPositionManager_TrailingStop(t *testing.T) {
	cfg := DefaultPositionConfig()
	pm := NewPositionManager(cfg, 10000)
	pm.OpenPosition("SOLUSDT", 1, 100.0, 1.0, 2.0, 1000)

	// Move price up to trigger phase 1 (50% of target distance = 3 ATR * 0.5 = 3)
	// Target is at 106, so 50% = 103
	pm.Update("SOLUSDT", 103.5, 2000, 0)

	pos := pm.positions["SOLUSDT"]
	if pos.Phase < 1 {
		t.Errorf("expected phase >= 1, got %d", pos.Phase)
	}
}

func TestPositionManager_ShortStop(t *testing.T) {
	pm := NewPositionManager(DefaultPositionConfig(), 10000)
	pm.OpenPosition("SOLUSDT", -1, 100.0, 1.0, 2.0, 1000)

	// Stop for short is 100 + 1.5*2 = 103
	ct := pm.Update("SOLUSDT", 104.0, 2000, 0.1)
	if ct == nil {
		t.Fatal("expected closed trade")
	}
	if ct.ExitReason != "stop" {
		t.Errorf("expected stop, got %s", ct.ExitReason)
	}
	if ct.PnLPct >= 0 {
		t.Error("expected negative P&L for short stopped out")
	}
}

func TestPositionManager_ShortTarget(t *testing.T) {
	// Use a config where phase 3 trigger is above 1.0 so target fires first.
	cfg := DefaultPositionConfig()
	cfg.Phase3Trigger = 1.5
	pm := NewPositionManager(cfg, 10000)
	pm.OpenPosition("SOLUSDT", -1, 100.0, 1.0, 2.0, 1000)

	// Target for short is 100 - 3*2 = 94
	ct := pm.Update("SOLUSDT", 93.0, 2000, 0.1)
	if ct == nil {
		t.Fatal("expected closed trade")
	}
	if ct.ExitReason != "target" {
		t.Errorf("expected target, got %s", ct.ExitReason)
	}
	if ct.PnLPct <= 0 {
		t.Error("expected positive P&L for short hitting target")
	}
}

func TestPositionManager_Timeout(t *testing.T) {
	cfg := DefaultPositionConfig()
	pm := NewPositionManager(cfg, 10000)
	pm.OpenPosition("SOLUSDT", 1, 100.0, 1.0, 2.0, 1000)

	// Exceed max holding period (4 hours = 14400000 ms)
	ct := pm.Update("SOLUSDT", 100.5, 1000+cfg.MaxHoldingMs+1, 0)
	if ct == nil {
		t.Fatal("expected closed trade on timeout")
	}
	if ct.ExitReason != "timeout" {
		t.Errorf("expected timeout, got %s", ct.ExitReason)
	}
}

func TestPositionManager_ComputeSize(t *testing.T) {
	pm := NewPositionManager(DefaultPositionConfig(), 10000)
	size := pm.ComputeSize(2.0, 100.0)
	if size <= 0 {
		t.Error("expected positive size")
	}
	// MaxRisk = 10000 * 0.005 = 50, riskPerUnit = 2.0 * 1.5 = 3.0
	// size = 50/3 * 0.25 = 4.17
	expected := (10000 * 0.005) / (2.0 * 1.5) * 0.25
	if math.Abs(size-expected) > 0.01 {
		t.Errorf("expected size %.2f, got %.2f", expected, size)
	}
}

func TestPositionManager_ComputeSizeZeroATR(t *testing.T) {
	pm := NewPositionManager(DefaultPositionConfig(), 10000)
	size := pm.ComputeSize(0, 100.0)
	if size != 0 {
		t.Errorf("expected 0 size for zero ATR, got %f", size)
	}
}

func TestPositionManager_HasPosition(t *testing.T) {
	pm := NewPositionManager(DefaultPositionConfig(), 10000)
	if pm.HasPosition("SOLUSDT") {
		t.Error("should not have position before opening")
	}
	pm.OpenPosition("SOLUSDT", 1, 100.0, 1.0, 2.0, 1000)
	if !pm.HasPosition("SOLUSDT") {
		t.Error("should have position after opening")
	}
}

func TestPositionManager_CloseAll(t *testing.T) {
	pm := NewPositionManager(DefaultPositionConfig(), 10000)
	pm.OpenPosition("SOLUSDT", 1, 100.0, 1.0, 2.0, 1000)
	pm.OpenPosition("BTCUSDT", -1, 50000.0, 0.1, 100.0, 1000)

	prices := map[string]float64{
		"SOLUSDT": 101.0,
		"BTCUSDT": 49900.0,
	}
	trades := pm.CloseAll(prices, 5000)
	if len(trades) != 2 {
		t.Errorf("expected 2 closed trades, got %d", len(trades))
	}
	if pm.HasPosition("SOLUSDT") || pm.HasPosition("BTCUSDT") {
		t.Error("all positions should be closed")
	}
}

func TestPositionManager_EquityUpdates(t *testing.T) {
	pm := NewPositionManager(DefaultPositionConfig(), 10000)
	pm.OpenPosition("SOLUSDT", 1, 100.0, 1.0, 2.0, 1000)

	// Stop out at 96 (below stop of 97) with 0 fees for predictable P&L
	ct := pm.Update("SOLUSDT", 96.0, 2000, 0)
	if ct == nil {
		t.Fatal("expected closed trade")
	}

	// PnL = (96-100)/100 * 1.0 * 100 = -4.0
	expectedEquity := 10000.0 + ct.PnLAbs - ct.Fees
	if math.Abs(pm.Equity()-expectedEquity) > 0.01 {
		t.Errorf("expected equity %.2f, got %.2f", expectedEquity, pm.Equity())
	}
	if pm.Equity() > 10000 {
		t.Error("equity should decrease after stop loss")
	}
}

func TestPositionManager_TrailingPhases(t *testing.T) {
	cfg := DefaultPositionConfig()
	pm := NewPositionManager(cfg, 10000)
	// ATR=2, entry=100, target=106 (6 away), stop=97
	pm.OpenPosition("SOLUSDT", 1, 100.0, 1.0, 2.0, 1000)

	// Phase 1 at 50% progress = price 103
	pm.Update("SOLUSDT", 103.5, 2000, 0)
	pos := pm.positions["SOLUSDT"]
	if pos.Phase != 1 {
		t.Errorf("expected phase 1, got %d", pos.Phase)
	}

	// Phase 2 at 80% progress = price 104.8
	pm.Update("SOLUSDT", 105.0, 3000, 0)
	pos = pm.positions["SOLUSDT"]
	if pos.Phase != 2 {
		t.Errorf("expected phase 2, got %d", pos.Phase)
	}

	// Phase 3 at 100% progress = price 106
	pm.Update("SOLUSDT", 106.5, 4000, 0)
	pos = pm.positions["SOLUSDT"]
	if pos.Phase != 3 {
		t.Errorf("expected phase 3, got %d", pos.Phase)
	}

	// Stop should now be tight: highWater(106.5) - 0.08*2 = 106.34
	expectedStop := 106.5 - cfg.Phase3Trail*2.0
	if math.Abs(pos.StopPrice-expectedStop) > 0.01 {
		t.Errorf("expected trailing stop %.4f, got %.4f", expectedStop, pos.StopPrice)
	}
}

func TestPositionManager_UpdateNoPosition(t *testing.T) {
	pm := NewPositionManager(DefaultPositionConfig(), 10000)
	ct := pm.Update("SOLUSDT", 100.0, 1000, 0)
	if ct != nil {
		t.Error("should return nil when no position exists")
	}
}
