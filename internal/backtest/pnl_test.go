package backtest

import (
	"math"
	"testing"
)

func TestPnLTracker_Metrics(t *testing.T) {
	tracker := NewPnLTracker(10000)
	for i := 0; i < 10; i++ {
		pnl := 50.0
		pnlPct := 0.005
		if i >= 6 {
			pnl = -30.0
			pnlPct = -0.003
		}
		tracker.Record(ClosedTrade{
			Symbol: "SOL", Direction: 1, EntryPrice: 100, ExitPrice: 100 + pnl/10,
			Size: 10, EntryTime: int64(i) * 86400000, ExitTime: int64(i)*86400000 + 3600000,
			PnLPct: pnlPct, PnLAbs: pnl, ExitReason: "target",
		})
	}
	m := tracker.Metrics()
	if m.TotalTrades != 10 {
		t.Errorf("expected 10, got %d", m.TotalTrades)
	}
	if math.Abs(m.WinRate-60) > 0.1 {
		t.Errorf("expected 60%% win rate, got %.1f", m.WinRate)
	}
	if m.ProfitFactor <= 0 {
		t.Error("expected positive profit factor")
	}
}

func TestPnLTracker_Empty(t *testing.T) {
	m := NewPnLTracker(10000).Metrics()
	if m.TotalTrades != 0 {
		t.Error("expected 0")
	}
}

func TestPnLTracker_Drawdown(t *testing.T) {
	tracker := NewPnLTracker(10000)
	tracker.Record(ClosedTrade{PnLAbs: 100, PnLPct: 0.01, ExitTime: 1000, Symbol: "X"})
	tracker.Record(ClosedTrade{PnLAbs: -200, PnLPct: -0.02, ExitTime: 2000, Symbol: "X"})
	m := tracker.Metrics()
	if m.MaxDrawdownPct <= 0 {
		t.Error("expected drawdown")
	}
}
