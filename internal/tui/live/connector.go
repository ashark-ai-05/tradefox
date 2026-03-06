package live

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
	"github.com/ashark-ai-05/tradefox/internal/tui/mock"
)

// ConnectionStatus represents the state of the exchange connection.
type ConnectionStatus int

const (
	StatusDisconnected ConnectionStatus = iota
	StatusConnecting
	StatusConnected
	StatusError
)

func (s ConnectionStatus) String() string {
	switch s {
	case StatusConnected:
		return "Connected"
	case StatusConnecting:
		return "Connecting"
	case StatusError:
		return "Error"
	default:
		return "Disconnected"
	}
}

// OrderBookLevel is a TUI-friendly order book price level.
type OrderBookLevel struct {
	Price float64
	Qty   float64
	IsBid bool
}

// TradeEvent is a TUI-friendly trade.
type TradeEvent struct {
	Symbol string
	Price  float64
	Size   float64
	Side   string // "BUY" or "SELL"
	Time   time.Time
}

// TickerUpdate is a TUI-friendly ticker snapshot.
type TickerUpdate struct {
	Symbol      string
	Price       float64
	Change24    float64
	Volume      float64
	Bid         float64
	Ask         float64
	FundingRate float64
}

// LiveDataBridge connects exchange connectors to TUI components via the event bus.
type LiveDataBridge struct {
	bus    *eventbus.Bus
	status ConnectionStatus
	mu     sync.RWMutex

	// Subscription IDs for cleanup
	obSubID    uint64
	tradeSubID uint64

	// Callbacks
	obCallbacks    map[string][]func(bids, asks []OrderBookLevel)
	tradeCallbacks map[string][]func(trade TradeEvent)
	tickerCallbacks map[string][]func(ticker TickerUpdate)
	signalCallbacks []func(signals mock.SignalSet)
	cbMu            sync.RWMutex

	candleCallbacks map[string][]func(candle Candle)

	useMock    bool
	publicFeed *BinancePublicFeed
}

// NewLiveDataBridge creates a bridge between the event bus and TUI.
// If bus is nil, mock data mode is used automatically.
func NewLiveDataBridge(bus *eventbus.Bus) *LiveDataBridge {
	b := &LiveDataBridge{
		bus:             bus,
		status:          StatusDisconnected,
		obCallbacks:     make(map[string][]func(bids, asks []OrderBookLevel)),
		tradeCallbacks:  make(map[string][]func(trade TradeEvent)),
		tickerCallbacks: make(map[string][]func(ticker TickerUpdate)),
		candleCallbacks: make(map[string][]func(candle Candle)),
		useMock:         bus == nil,
	}
	if b.useMock {
		b.status = StatusDisconnected
	}
	return b
}

// Start begins listening on the event bus. Call in a goroutine.
func (b *LiveDataBridge) Start(ctx context.Context) {
	if b.useMock {
		b.startMockFeed(ctx)
		return
	}

	b.mu.Lock()
	b.status = StatusConnecting
	b.mu.Unlock()

	// Subscribe to order book updates
	obID, obCh := b.bus.OrderBooks.Subscribe(64)
	b.mu.Lock()
	b.obSubID = obID
	b.status = StatusConnected
	b.mu.Unlock()

	// Subscribe to trade updates
	tradeID, tradeCh := b.bus.Trades.Subscribe(256)
	b.mu.Lock()
	b.tradeSubID = tradeID
	b.mu.Unlock()

	go b.processOrderBooks(ctx, obCh)
	go b.processTrades(ctx, tradeCh)

	<-ctx.Done()
	b.bus.OrderBooks.Unsubscribe(obID)
	b.bus.Trades.Unsubscribe(tradeID)
}

func (b *LiveDataBridge) processOrderBooks(ctx context.Context, ch <-chan *models.OrderBook) {
	for {
		select {
		case <-ctx.Done():
			return
		case ob, ok := <-ch:
			if !ok {
				return
			}
			bids := ob.Bids()
			asks := ob.Asks()
			var tuiBids, tuiAsks []OrderBookLevel
			for _, bid := range bids {
				if bid.Price != nil && bid.Size != nil {
					tuiBids = append(tuiBids, OrderBookLevel{Price: *bid.Price, Qty: *bid.Size, IsBid: true})
				}
			}
			for _, ask := range asks {
				if ask.Price != nil && ask.Size != nil {
					tuiAsks = append(tuiAsks, OrderBookLevel{Price: *ask.Price, Qty: *ask.Size, IsBid: false})
				}
			}
			b.cbMu.RLock()
			for _, cb := range b.obCallbacks[ob.Symbol] {
				cb(tuiBids, tuiAsks)
			}
			b.cbMu.RUnlock()
		}
	}
}

func (b *LiveDataBridge) processTrades(ctx context.Context, ch <-chan models.Trade) {
	for {
		select {
		case <-ctx.Done():
			return
		case t, ok := <-ch:
			if !ok {
				return
			}
			side := "BUY"
			if t.IsBuy != nil && !*t.IsBuy {
				side = "SELL"
			}
			evt := TradeEvent{
				Symbol: t.Symbol,
				Price:  t.Price.InexactFloat64(),
				Size:   t.Size.InexactFloat64(),
				Side:   side,
				Time:   t.Timestamp,
			}
			b.cbMu.RLock()
			for _, cb := range b.tradeCallbacks[t.Symbol] {
				cb(evt)
			}
			b.cbMu.RUnlock()
		}
	}
}

func (b *LiveDataBridge) startMockFeed(ctx context.Context) {
	b.mu.Lock()
	b.status = StatusDisconnected
	b.mu.Unlock()

	// Mock mode: no live feed, TUI uses static mock data
	<-ctx.Done()
}

// SubscribeOrderBook registers a callback for order book updates on a symbol.
func (b *LiveDataBridge) SubscribeOrderBook(symbol string, callback func(bids, asks []OrderBookLevel)) {
	b.cbMu.Lock()
	defer b.cbMu.Unlock()
	b.obCallbacks[symbol] = append(b.obCallbacks[symbol], callback)
}

// SubscribeTrades registers a callback for trade events on a symbol.
func (b *LiveDataBridge) SubscribeTrades(symbol string, callback func(trade TradeEvent)) {
	b.cbMu.Lock()
	defer b.cbMu.Unlock()
	b.tradeCallbacks[symbol] = append(b.tradeCallbacks[symbol], callback)
}

// SubscribeTicker registers a callback for ticker updates on a symbol.
func (b *LiveDataBridge) SubscribeTicker(symbol string, callback func(ticker TickerUpdate)) {
	b.cbMu.Lock()
	defer b.cbMu.Unlock()
	b.tickerCallbacks[symbol] = append(b.tickerCallbacks[symbol], callback)
}

// SubscribeSignals registers a callback for signal updates.
func (b *LiveDataBridge) SubscribeSignals(callback func(signals mock.SignalSet)) {
	b.cbMu.Lock()
	defer b.cbMu.Unlock()
	b.signalCallbacks = append(b.signalCallbacks, callback)
}

// Status returns the current connection status.
func (b *LiveDataBridge) Status() ConnectionStatus {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.status
}

// SubscribeCandles registers a callback for candle updates on a symbol.
func (b *LiveDataBridge) SubscribeCandles(symbol string, callback func(candle Candle)) {
	b.cbMu.Lock()
	defer b.cbMu.Unlock()
	b.candleCallbacks[symbol] = append(b.candleCallbacks[symbol], callback)
}

// ConnectPublic connects to Binance Futures public WebSocket (no API keys needed).
func (b *LiveDataBridge) ConnectPublic(ctx context.Context, symbol string) error {
	b.mu.Lock()
	b.status = StatusConnecting
	b.useMock = false
	b.mu.Unlock()

	feed := NewBinancePublicFeed(symbol)
	b.mu.Lock()
	b.publicFeed = feed
	b.mu.Unlock()

	upperSymbol := feed.Symbol()

	feed.OnTrade(func(evt TradeEvent) {
		b.cbMu.RLock()
		for _, cb := range b.tradeCallbacks[upperSymbol] {
			cb(evt)
		}
		b.cbMu.RUnlock()
	})

	feed.OnBook(func(bids, asks []OrderBookLevel) {
		b.cbMu.RLock()
		for _, cb := range b.obCallbacks[upperSymbol] {
			cb(bids, asks)
		}
		b.cbMu.RUnlock()
	})

	feed.OnTicker(func(ticker TickerUpdate) {
		b.cbMu.RLock()
		for _, cb := range b.tickerCallbacks[upperSymbol] {
			cb(ticker)
		}
		b.cbMu.RUnlock()
	})

	feed.OnCandle(func(candle Candle) {
		b.cbMu.RLock()
		for _, cb := range b.candleCallbacks[upperSymbol] {
			cb(candle)
		}
		b.cbMu.RUnlock()
	})

	fmt.Fprintf(os.Stderr, "TradeFox: callbacks registered for %s\n", upperSymbol)

	if err := feed.Connect(ctx); err != nil {
		b.mu.Lock()
		b.status = StatusError
		b.mu.Unlock()
		return err
	}

	b.mu.Lock()
	b.status = StatusConnected
	b.mu.Unlock()

	return nil
}

// Close cleans up the public feed connection.
func (b *LiveDataBridge) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.publicFeed != nil {
		b.publicFeed.Close()
		b.publicFeed = nil
	}
}

// IsMock returns true if running in mock data mode.
func (b *LiveDataBridge) IsMock() bool {
	return b.useMock
}
