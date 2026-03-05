package backtest

import (
	"math"
	"time"
)

type BacktestMetrics struct {
	TotalTrades    int                      `json:"totalTrades"`
	WinRate        float64                  `json:"winRate"`
	AvgWinPct      float64                  `json:"avgWinPct"`
	AvgLossPct     float64                  `json:"avgLossPct"`
	ProfitFactor   float64                  `json:"profitFactor"`
	SharpeDaily    float64                  `json:"sharpeDaily"`
	MaxDrawdownPct float64                  `json:"maxDrawdownPct"`
	TotalReturnPct float64                  `json:"totalReturnPct"`
	AvgHoldingMs   int64                    `json:"avgHoldingMs"`
	TradesPerDay   float64                  `json:"tradesPerDay"`
	BySymbol       map[string]SymbolMetrics `json:"bySymbol"`
}

type SymbolMetrics struct {
	Trades   int     `json:"trades"`
	WinRate  float64 `json:"winRate"`
	TotalPnL float64 `json:"totalPnl"`
}

type PnLTracker struct {
	Trades        []ClosedTrade
	initialEquity float64
	equity        float64
	peakEquity    float64
	maxDrawdown   float64
	dailyReturns  map[string]float64
}

func NewPnLTracker(initialEquity float64) *PnLTracker {
	return &PnLTracker{
		initialEquity: initialEquity,
		equity:        initialEquity,
		peakEquity:    initialEquity,
		dailyReturns:  make(map[string]float64),
	}
}

func (p *PnLTracker) Record(trade ClosedTrade) {
	p.Trades = append(p.Trades, trade)
	p.equity += trade.PnLAbs - trade.Fees
	if p.equity > p.peakEquity {
		p.peakEquity = p.equity
	}
	dd := (p.peakEquity - p.equity) / p.peakEquity
	if dd > p.maxDrawdown {
		p.maxDrawdown = dd
	}
	day := time.UnixMilli(trade.ExitTime).UTC().Format("2006-01-02")
	p.dailyReturns[day] += trade.PnLPct
}

func (p *PnLTracker) Metrics() BacktestMetrics {
	m := BacktestMetrics{
		TotalTrades:    len(p.Trades),
		MaxDrawdownPct: p.maxDrawdown * 100,
		TotalReturnPct: ((p.equity - p.initialEquity) / p.initialEquity) * 100,
		BySymbol:       make(map[string]SymbolMetrics),
	}
	if len(p.Trades) == 0 {
		return m
	}

	var wins, losses int
	var totalWinPct, totalLossPct, grossProfit, grossLoss float64
	var totalHoldingMs int64

	for _, t := range p.Trades {
		if t.PnLPct > 0 {
			wins++
			totalWinPct += t.PnLPct
			grossProfit += t.PnLAbs
		} else {
			losses++
			totalLossPct += t.PnLPct
			grossLoss += math.Abs(t.PnLAbs)
		}
		totalHoldingMs += t.ExitTime - t.EntryTime

		sm := m.BySymbol[t.Symbol]
		sm.Trades++
		if t.PnLPct > 0 {
			sm.WinRate++
		}
		sm.TotalPnL += t.PnLAbs
		m.BySymbol[t.Symbol] = sm
	}

	m.WinRate = float64(wins) / float64(len(p.Trades)) * 100
	if wins > 0 {
		m.AvgWinPct = (totalWinPct / float64(wins)) * 100
	}
	if losses > 0 {
		m.AvgLossPct = (totalLossPct / float64(losses)) * 100
	}
	if grossLoss > 0 {
		m.ProfitFactor = grossProfit / grossLoss
	}
	m.AvgHoldingMs = totalHoldingMs / int64(len(p.Trades))

	if len(p.dailyReturns) > 1 {
		var rets []float64
		for _, r := range p.dailyReturns {
			rets = append(rets, r)
		}
		mean, stddev := meanStdDev(rets)
		if stddev > 0 {
			m.SharpeDaily = (mean / stddev) * math.Sqrt(365)
		}
	}

	first := p.Trades[0].EntryTime
	last := p.Trades[len(p.Trades)-1].ExitTime
	days := float64(last-first) / (24 * 60 * 60 * 1000)
	if days > 0 {
		m.TradesPerDay = float64(len(p.Trades)) / days
	}

	for sym, sm := range m.BySymbol {
		if sm.Trades > 0 {
			sm.WinRate = sm.WinRate / float64(sm.Trades) * 100
		}
		m.BySymbol[sym] = sm
	}

	return m
}

func meanStdDev(xs []float64) (mean, stddev float64) {
	n := float64(len(xs))
	if n == 0 {
		return
	}
	for _, x := range xs {
		mean += x
	}
	mean /= n
	var variance float64
	for _, x := range xs {
		d := x - mean
		variance += d * d
	}
	variance /= n
	stddev = math.Sqrt(variance)
	return
}
