package backtest

import "fmt"

type OverfitCheck struct {
	Flags   []string `json:"flags"`
	Healthy []string `json:"healthy"`
	Verdict string   `json:"verdict"`
}

func CheckOverfitting(result *BacktestResult) *OverfitCheck {
	check := &OverfitCheck{}
	m := result.Metrics

	if m.SharpeDaily > 4.0 {
		check.Flags = append(check.Flags, fmt.Sprintf("Sharpe %.1f > 4.0 (suspiciously high)", m.SharpeDaily))
	}
	if m.WinRate > 80 {
		check.Flags = append(check.Flags, fmt.Sprintf("Win rate %.1f%% > 80%% (likely overfit)", m.WinRate))
	}
	if m.TotalTrades < 50 {
		check.Flags = append(check.Flags, fmt.Sprintf("Only %d trades (insufficient sample)", m.TotalTrades))
	}

	if m.SharpeDaily >= 1.5 && m.SharpeDaily <= 3.0 {
		check.Healthy = append(check.Healthy, fmt.Sprintf("Sharpe %.1f in healthy range", m.SharpeDaily))
	}
	if m.WinRate >= 62 && m.WinRate <= 72 {
		check.Healthy = append(check.Healthy, fmt.Sprintf("Win rate %.1f%% in healthy range", m.WinRate))
	}
	if m.TotalTrades >= 100 {
		check.Healthy = append(check.Healthy, fmt.Sprintf("%d trades - good sample size", m.TotalTrades))
	}
	if m.MaxDrawdownPct < 10 {
		check.Healthy = append(check.Healthy, fmt.Sprintf("Max DD %.1f%% is manageable", m.MaxDrawdownPct))
	}

	if len(check.Flags) == 0 {
		check.Verdict = "healthy"
	} else if len(check.Flags) <= 1 {
		check.Verdict = "warning"
	} else {
		check.Verdict = "danger"
	}

	return check
}
