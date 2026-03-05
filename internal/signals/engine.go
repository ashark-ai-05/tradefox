package signals

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

// Engine computes microstructure signals per symbol from the event bus.
type Engine struct {
	bus    *eventbus.Bus
	logger *slog.Logger

	mu     sync.RWMutex
	states map[string]*SymbolState
	latest map[string]*SignalSet

	onUpdate func(symbol string, signals *SignalSet)
}

// SymbolState holds per-symbol state for signal computation.
type SymbolState struct {
	OfiState  OFIState
	Smoothed  SmoothedValues
	VolState  VolState
	PrevBids  LevelMap
	PrevAsks  LevelMap
	Trades    []TradeRecord
	MaxTrades int
}

// SmoothedValues holds EMA-smoothed signal values.
type SmoothedValues struct {
	DivBps    float64
	Ofi       float64
	Weighted  float64
	Lambda    float64
	Vol       float64
	Composite float64
}

// NewSymbolState creates a new SymbolState with default settings.
func NewSymbolState() *SymbolState {
	return &SymbolState{MaxTrades: 200}
}

// NewEngine creates a signal engine.
func NewEngine(bus *eventbus.Bus, logger *slog.Logger) *Engine {
	return &Engine{
		bus:    bus,
		logger: logger,
		states: make(map[string]*SymbolState),
		latest: make(map[string]*SignalSet),
	}
}

// OnUpdate registers a callback invoked when signals are recomputed.
func (e *Engine) OnUpdate(fn func(symbol string, signals *SignalSet)) {
	e.onUpdate = fn
}

// Latest returns the most recent SignalSet for a symbol.
func (e *Engine) Latest(symbol string) *SignalSet {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.latest[symbol]
}

// Start subscribes to event bus topics and begins computing signals.
func (e *Engine) Start(ctx context.Context) {
	obID, obCh := e.bus.OrderBooks.Subscribe(256)
	trID, trCh := e.bus.Trades.Subscribe(1024)

	go func() {
		defer e.bus.OrderBooks.Unsubscribe(obID)
		defer e.bus.Trades.Unsubscribe(trID)
		for {
			select {
			case <-ctx.Done():
				return
			case ob, ok := <-obCh:
				if !ok {
					return
				}
				e.handleOrderBook(ob)
			case t, ok := <-trCh:
				if !ok {
					return
				}
				e.handleTrade(t)
			}
		}
	}()

	e.logger.Info("signal engine started")
}

func (e *Engine) getState(symbol string) *SymbolState {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, ok := e.states[symbol]
	if !ok {
		s = NewSymbolState()
		e.states[symbol] = s
	}
	return s
}

func (e *Engine) handleTrade(t models.Trade) {
	s := e.getState(t.Symbol)
	price, _ := t.Price.Float64()
	size, _ := t.Size.Float64()
	isBuy := false
	if t.IsBuy != nil {
		isBuy = *t.IsBuy
	}
	s.Trades = append(s.Trades, TradeRecord{
		Price:     price,
		Size:      size,
		IsBuy:     isBuy,
		Timestamp: t.Timestamp,
	})
	if len(s.Trades) > s.MaxTrades {
		s.Trades = s.Trades[len(s.Trades)-s.MaxTrades:]
	}
}

func (e *Engine) handleOrderBook(ob *models.OrderBook) {
	s := e.getState(ob.Symbol)
	bids := ob.Bids()
	asks := ob.Asks()
	bidLevels := toBookLevels(bids)
	askLevels := toBookLevels(asks)
	mid := ob.MidPrice()
	micro := ob.MicroPrice()

	sigs := ComputeAll(s, bidLevels, askLevels, mid, micro)

	e.mu.Lock()
	e.latest[ob.Symbol] = sigs
	e.mu.Unlock()

	if e.onUpdate != nil {
		e.onUpdate(ob.Symbol, sigs)
	}
}

// ComputeAll runs all 8 signals on the given state and returns the SignalSet.
// This is the core computation shared by the live engine, validation replayer, and backtest.
func ComputeAll(s *SymbolState, bidLevels, askLevels []BookLevel, mid, micro float64) *SignalSet {
	// Microprice
	microSig := ComputeMicroprice(mid, micro, s.Smoothed.DivBps)
	s.Smoothed.DivBps = microSig.DivBps

	// OFI
	bidPrice, bidSize, askPrice, askSize := 0.0, 0.0, 0.0, 0.0
	if len(bidLevels) > 0 {
		bidPrice = bidLevels[0].Price
		bidSize = bidLevels[0].Size
	}
	if len(askLevels) > 0 {
		askPrice = askLevels[0].Price
		askSize = askLevels[0].Size
	}
	ofiSig, newOFIState := ComputeOFI(s.OfiState, bidPrice, bidSize, askPrice, askSize, s.Smoothed.Ofi)
	s.OfiState = newOFIState
	s.Smoothed.Ofi = ofiSig.Value

	// Depth
	depthSig := ComputeDepthImbalance(bidLevels, askLevels, s.Smoothed.Weighted)
	s.Smoothed.Weighted = depthSig.Weighted

	// Sweep (use time of last trade or time.Now() as fallback)
	now := time.Now()
	if len(s.Trades) > 0 {
		now = s.Trades[len(s.Trades)-1].Timestamp
	}
	sweepSig := ComputeSweepAt(s.Trades, now)

	// Lambda
	lambdaSig := ComputeLambda(s.Trades, s.Smoothed.Lambda)
	s.Smoothed.Lambda = lambdaSig.Value

	// Volatility
	volSig := ComputeVolatility(s.Trades, s.Smoothed.Vol, &s.VolState)
	s.Smoothed.Vol = volSig.Realized

	// Spoof
	spoofSig := ComputeSpoof(s.PrevBids, s.PrevAsks, bidLevels, askLevels)
	s.PrevBids = BuildLevelMap(bidLevels)
	s.PrevAsks = BuildLevelMap(askLevels)

	// Composite
	compSig := ComputeComposite(microSig, ofiSig, depthSig, sweepSig, s.Smoothed.Composite)
	s.Smoothed.Composite = compSig.Avg

	return &SignalSet{
		Microprice: microSig,
		OFI:        ofiSig,
		DepthImb:   depthSig,
		Sweep:      sweepSig,
		Lambda:     lambdaSig,
		Vol:        volSig,
		Spoof:      spoofSig,
		Composite:  compSig,
	}
}

func toBookLevels(items []models.BookItem) []BookLevel {
	levels := make([]BookLevel, 0, len(items))
	for _, item := range items {
		if item.Price != nil && item.Size != nil {
			levels = append(levels, BookLevel{Price: *item.Price, Size: *item.Size})
		}
	}
	return levels
}
