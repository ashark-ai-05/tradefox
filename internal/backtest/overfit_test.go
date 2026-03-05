package backtest

import "testing"

func TestOverfitCheck_Healthy(t *testing.T) {
	check := CheckOverfitting(&BacktestResult{Metrics: BacktestMetrics{TotalTrades: 150, WinRate: 65, SharpeDaily: 2.0, MaxDrawdownPct: 5}})
	if check.Verdict != "healthy" {
		t.Errorf("expected healthy, got %s", check.Verdict)
	}
}

func TestOverfitCheck_Danger(t *testing.T) {
	check := CheckOverfitting(&BacktestResult{Metrics: BacktestMetrics{TotalTrades: 20, WinRate: 85, SharpeDaily: 5.0}})
	if check.Verdict != "danger" {
		t.Errorf("expected danger, got %s", check.Verdict)
	}
}
