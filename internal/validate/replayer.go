package validate

import (
	"strconv"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/recorder"
	"github.com/ashark-ai-05/tradefox/internal/replay"
	"github.com/ashark-ai-05/tradefox/internal/signals"
)

// SignalSnapshot captures signals + price at a point in time.
type SignalSnapshot struct {
	Timestamp int64
	Symbol    string
	Signals   *signals.SignalSet
	MidPrice  float64
}

// Replayer feeds replay records through signal computation.
type Replayer struct {
	states map[string]*signals.SymbolState
}

func NewReplayer() *Replayer {
	return &Replayer{states: make(map[string]*signals.SymbolState)}
}

func (r *Replayer) getState(symbol string) *signals.SymbolState {
	s, ok := r.states[symbol]
	if !ok {
		s = signals.NewSymbolState()
		r.states[symbol] = s
	}
	return s
}

// Process feeds a record and returns a SignalSnapshot if it was an OB update.
func (r *Replayer) Process(rec replay.Record) *SignalSnapshot {
	switch {
	case rec.Trade != nil:
		r.handleTrade(rec.Trade)
		return nil
	case rec.OB != nil:
		return r.handleOB(rec.OB)
	default:
		return nil // kiyotaka records don't produce signals
	}
}

func (r *Replayer) handleTrade(t *recorder.TradeRecord) {
	s := r.getState(t.Symbol)
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

func (r *Replayer) handleOB(ob *recorder.OrderBookRecord) *SignalSnapshot {
	s := r.getState(ob.Symbol)

	bidLevels := make([]signals.BookLevel, len(ob.Bids))
	for i, b := range ob.Bids {
		bidLevels[i] = signals.BookLevel{Price: b.Price, Size: b.Size}
	}
	askLevels := make([]signals.BookLevel, len(ob.Asks))
	for i, a := range ob.Asks {
		askLevels[i] = signals.BookLevel{Price: a.Price, Size: a.Size}
	}

	sigs := signals.ComputeAll(s, bidLevels, askLevels, ob.MidPrice, ob.MicroPrice)

	return &SignalSnapshot{
		Timestamp: ob.LocalTS,
		Symbol:    ob.Symbol,
		Signals:   sigs,
		MidPrice:  ob.MidPrice,
	}
}
