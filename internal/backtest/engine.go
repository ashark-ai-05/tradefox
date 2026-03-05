package backtest

import (
	"log/slog"
	"sort"

	"github.com/ashark-ai-05/tradefox/internal/replay"
)

type EngineConfig struct {
	Strategy      StrategyConfig  `json:"strategy"`
	Position      PositionConfig  `json:"position"`
	Execution     ExecutionConfig `json:"execution"`
	InitialEquity float64         `json:"initialEquity"`
}

func DefaultEngineConfig() EngineConfig {
	return EngineConfig{
		Strategy:      DefaultStrategyConfig(),
		Position:      DefaultPositionConfig(),
		Execution:     DefaultExecutionConfig(),
		InitialEquity: 10000.0,
	}
}

type BacktestResult struct {
	Metrics   BacktestMetrics `json:"metrics"`
	Trades    []ClosedTrade   `json:"trades"`
	Config    EngineConfig    `json:"config"`
	DataStats DataStats       `json:"dataStats"`
}

type DataStats struct {
	TotalRecords int64    `json:"totalRecords"`
	OBRecords    int64    `json:"obRecords"`
	TradeRecords int64    `json:"tradeRecords"`
	KiyRecords   int64    `json:"kiyRecords"`
	StartTime    int64    `json:"startTime"`
	EndTime      int64    `json:"endTime"`
	Symbols      []string `json:"symbols"`
}

type Engine struct {
	config     EngineConfig
	feed       *SignalFeed
	positions  *PositionManager
	execution  map[string]*ExecutionSimulator
	pnl        *PnLTracker
	atr        map[string]*ATRCalculator
	ofiTracker *OFITracker
	logger     *slog.Logger
	lastPrices map[string]float64

	// Store pending direction so we know which way to open when fill occurs
	pendingDir map[string]int
}

func NewEngine(cfg EngineConfig, logger *slog.Logger) *Engine {
	return &Engine{
		config:     cfg,
		feed:       NewSignalFeed(),
		positions:  NewPositionManager(cfg.Position, cfg.InitialEquity),
		execution:  make(map[string]*ExecutionSimulator),
		pnl:        NewPnLTracker(cfg.InitialEquity),
		atr:        make(map[string]*ATRCalculator),
		ofiTracker: NewOFITracker(),
		logger:     logger,
		lastPrices: make(map[string]float64),
		pendingDir: make(map[string]int),
	}
}

func (e *Engine) getExec(symbol string) *ExecutionSimulator {
	exec, ok := e.execution[symbol]
	if !ok {
		exec = NewExecutionSimulator(e.config.Execution)
		e.execution[symbol] = exec
	}
	return exec
}

func (e *Engine) getATR(symbol string) *ATRCalculator {
	atr, ok := e.atr[symbol]
	if !ok {
		atr = NewATRCalculator(14)
		e.atr[symbol] = atr
	}
	return atr
}

func (e *Engine) Run(records []replay.Record) (*BacktestResult, error) {
	stats := e.computeStats(records)

	for _, rec := range records {
		// Update ATR from kiyotaka OHLCV candles
		if rec.Kiy != nil && rec.Kiy.Type == "ohlcv" {
			e.getATR(rec.Kiy.Symbol).Update(rec.Kiy.High, rec.Kiy.Low, rec.Kiy.Close)
		}

		// Feed record to signal engine
		evt := e.feed.Process(rec)
		if evt == nil {
			continue
		}

		e.lastPrices[evt.Symbol] = evt.MidPrice
		exec := e.getExec(evt.Symbol)

		// 1. Try to fill pending order
		if exec.HasPending() {
			fill := exec.TryFill(evt.MidPrice, evt.Spread, evt.Signals.Lambda.Value, evt.Timestamp)
			if fill != nil {
				dir := e.pendingDir[evt.Symbol]
				atr := e.getATR(evt.Symbol)
				e.positions.OpenPosition(evt.Symbol, dir, fill.Price, fill.Size, atr.Current(), fill.Timestamp)
			}
		}

		// 2. Update open positions (stops/targets/trailing)
		if e.positions.HasPosition(evt.Symbol) {
			ct := e.positions.Update(evt.Symbol, evt.MidPrice, evt.Timestamp, 0)
			if ct != nil {
				// Apply exit fees
				notional := ct.ExitPrice * ct.Size
				ct.Fees += notional * e.config.Execution.TakerFeeBps / 10000
				e.pnl.Record(*ct)
			}
		}

		// 3. Evaluate entry (no position, no pending order)
		if !e.positions.HasPosition(evt.Symbol) && !exec.HasPending() {
			ofiPersistence := e.ofiTracker.Update(evt.Symbol, evt.Signals.OFI.Value)
			confluence := ComputeConfluence(e.config.Strategy, *evt, ofiPersistence)

			if !confluence.Vetoed && confluence.Score >= e.config.Strategy.ConfluenceThreshold && confluence.Direction != 0 {
				atr := e.getATR(evt.Symbol)
				if atr.Current() > 0 {
					size := e.positions.ComputeSize(atr.Current(), evt.MidPrice)
					if size > 0 {
						exec.PlaceOrder(evt.Symbol, confluence.Direction, size, evt.Timestamp)
						e.pendingDir[evt.Symbol] = confluence.Direction
					}
				}
			}
		}
	}

	// Close remaining positions
	for _, ct := range e.positions.CloseAll(e.lastPrices, stats.EndTime) {
		e.pnl.Record(ct)
	}

	return &BacktestResult{
		Metrics:   e.pnl.Metrics(),
		Trades:    e.pnl.Trades,
		Config:    e.config,
		DataStats: stats,
	}, nil
}

func (e *Engine) computeStats(records []replay.Record) DataStats {
	stats := DataStats{TotalRecords: int64(len(records))}
	symbols := map[string]bool{}
	for _, r := range records {
		if stats.StartTime == 0 || r.LocalTS < stats.StartTime {
			stats.StartTime = r.LocalTS
		}
		if r.LocalTS > stats.EndTime {
			stats.EndTime = r.LocalTS
		}
		switch {
		case r.OB != nil:
			stats.OBRecords++
			symbols[r.OB.Symbol] = true
		case r.Trade != nil:
			stats.TradeRecords++
			symbols[r.Trade.Symbol] = true
		case r.Kiy != nil:
			stats.KiyRecords++
			symbols[r.Kiy.Symbol] = true
		}
	}
	for s := range symbols {
		stats.Symbols = append(stats.Symbols, s)
	}
	sort.Strings(stats.Symbols)
	return stats
}
