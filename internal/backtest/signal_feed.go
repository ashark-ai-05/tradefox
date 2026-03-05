package backtest

import (
	"strconv"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/recorder"
	"github.com/ashark-ai-05/tradefox/internal/replay"
	"github.com/ashark-ai-05/tradefox/internal/signals"
)

// SignalEvent is produced when an orderbook update triggers signal computation.
type SignalEvent struct {
	Timestamp int64
	Symbol    string
	Signals   *signals.SignalSet
	MidPrice  float64
	BestBid   float64
	BestAsk   float64
	Spread    float64
}

// SignalFeed replays recorded data and produces SignalEvents.
type SignalFeed struct {
	states map[string]*signals.SymbolState
}

func NewSignalFeed() *SignalFeed {
	return &SignalFeed{states: make(map[string]*signals.SymbolState)}
}

func (f *SignalFeed) getState(symbol string) *signals.SymbolState {
	s, ok := f.states[symbol]
	if !ok {
		s = signals.NewSymbolState()
		f.states[symbol] = s
	}
	return s
}

// Process feeds a record and returns a SignalEvent if it was an OB update.
func (f *SignalFeed) Process(rec replay.Record) *SignalEvent {
	switch {
	case rec.Trade != nil:
		f.handleTrade(rec.Trade)
		return nil
	case rec.OB != nil:
		return f.handleOB(rec.OB)
	default:
		return nil
	}
}

func (f *SignalFeed) handleTrade(t *recorder.TradeRecord) {
	s := f.getState(t.Symbol)
	price, _ := strconv.ParseFloat(t.Price, 64)
	size, _ := strconv.ParseFloat(t.Size, 64)
	isBuy := false
	if t.IsBuy != nil {
		isBuy = *t.IsBuy
	}
	s.Trades = append(s.Trades, signals.TradeRecord{
		Price:     price,
		Size:      size,
		IsBuy:     isBuy,
		Timestamp: time.UnixMilli(t.ExchangeTS),
	})
	if len(s.Trades) > s.MaxTrades {
		s.Trades = s.Trades[len(s.Trades)-s.MaxTrades:]
	}
}

func (f *SignalFeed) handleOB(ob *recorder.OrderBookRecord) *SignalEvent {
	s := f.getState(ob.Symbol)

	bidLevels := make([]signals.BookLevel, len(ob.Bids))
	for i, b := range ob.Bids {
		bidLevels[i] = signals.BookLevel{Price: b.Price, Size: b.Size}
	}
	askLevels := make([]signals.BookLevel, len(ob.Asks))
	for i, a := range ob.Asks {
		askLevels[i] = signals.BookLevel{Price: a.Price, Size: a.Size}
	}

	sigs := signals.ComputeAll(s, bidLevels, askLevels, ob.MidPrice, ob.MicroPrice)

	bestBid, bestAsk := 0.0, 0.0
	if len(ob.Bids) > 0 {
		bestBid = ob.Bids[0].Price
	}
	if len(ob.Asks) > 0 {
		bestAsk = ob.Asks[0].Price
	}

	return &SignalEvent{
		Timestamp: ob.LocalTS,
		Symbol:    ob.Symbol,
		Signals:   sigs,
		MidPrice:  ob.MidPrice,
		BestBid:   bestBid,
		BestAsk:   bestAsk,
		Spread:    ob.Spread,
	}
}
