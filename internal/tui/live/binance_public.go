package live

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Candle represents a single OHLCV candlestick from live data.
type Candle struct {
	Time   int64
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

// BinancePublicFeed connects to Binance Futures public WebSocket streams.
// No API keys needed — all endpoints are publicly available.
type BinancePublicFeed struct {
	conn   *websocket.Conn
	symbol string // lowercase, e.g. "btcusdt"

	callbacks struct {
		onTrade  func(TradeEvent)
		onBook   func(bids, asks []OrderBookLevel)
		onTicker func(TickerUpdate)
		onCandle func(Candle)
	}

	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	reconnectAttempt int
	msgOnce          sync.Once
}

// NewBinancePublicFeed creates a new feed for the given symbol (case-insensitive).
func NewBinancePublicFeed(symbol string) *BinancePublicFeed {
	return &BinancePublicFeed{
		symbol: strings.ToLower(symbol),
		done:   make(chan struct{}),
	}
}

// OnTrade sets the callback for aggTrade events.
func (f *BinancePublicFeed) OnTrade(cb func(TradeEvent)) {
	f.mu.Lock()
	f.callbacks.onTrade = cb
	f.mu.Unlock()
}

// OnBook sets the callback for order book depth updates.
func (f *BinancePublicFeed) OnBook(cb func(bids, asks []OrderBookLevel)) {
	f.mu.Lock()
	f.callbacks.onBook = cb
	f.mu.Unlock()
}

// OnTicker sets the callback for mark price / ticker updates.
func (f *BinancePublicFeed) OnTicker(cb func(TickerUpdate)) {
	f.mu.Lock()
	f.callbacks.onTicker = cb
	f.mu.Unlock()
}

// OnCandle sets the callback for kline updates.
func (f *BinancePublicFeed) OnCandle(cb func(Candle)) {
	f.mu.Lock()
	f.callbacks.onCandle = cb
	f.mu.Unlock()
}

// Connect establishes the WebSocket connection and starts reading.
func (f *BinancePublicFeed) Connect(ctx context.Context) error {
	f.ctx, f.cancel = context.WithCancel(ctx)

	if err := f.dial(); err != nil {
		return err
	}

	go f.readLoop()
	return nil
}

func (f *BinancePublicFeed) dial() error {
	streams := []string{
		f.symbol + "@aggTrade",
		f.symbol + "@depth20@100ms",
		f.symbol + "@markPrice@1s",
		f.symbol + "@kline_1m",
	}

	u := url.URL{
		Scheme:   "wss",
		Host:     "fstream.binance.com",
		Path:     "/stream",
		RawQuery: "streams=" + strings.Join(streams, "/"),
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(f.ctx, u.String(), nil)
	if err != nil {
		return fmt.Errorf("binance ws dial: %w", err)
	}

	// Set ping/pong handler — Binance sends pings, we must respond with pong.
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})
	conn.SetPingHandler(func(msg string) error {
		_ = conn.WriteControl(websocket.PongMessage, []byte(msg), time.Now().Add(5*time.Second))
		return conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	fmt.Fprintf(os.Stderr, "TradeFox: WebSocket connected to %s\n", u.String())

	f.mu.Lock()
	f.conn = conn
	f.reconnectAttempt = 0
	f.mu.Unlock()

	return nil
}

// readLoop reads messages from the WebSocket and dispatches to callbacks.
func (f *BinancePublicFeed) readLoop() {
	defer close(f.done)

	for {
		select {
		case <-f.ctx.Done():
			return
		default:
		}

		f.mu.RLock()
		conn := f.conn
		f.mu.RUnlock()

		if conn == nil {
			return
		}

		_, msg, err := conn.ReadMessage()
		if err != nil {
			if f.ctx.Err() != nil {
				return
			}
			fmt.Fprintf(os.Stderr, "TradeFox: WebSocket error: %v\n", err)
			// Connection lost — attempt reconnect.
			f.reconnect()
			continue
		}

		// Reset read deadline on successful read.
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		f.handleMessage(msg)
	}
}

// Combined stream message wrapper: {"stream":"btcusdt@aggTrade","data":{...}}
type combinedStreamMsg struct {
	Stream string          `json:"stream"`
	Data   json.RawMessage `json:"data"`
}

type aggTradeMsg struct {
	Event     string `json:"e"`
	EventTime int64  `json:"E"`
	Symbol    string `json:"s"`
	Price     string `json:"p"`
	Quantity  string `json:"q"`
	IsMaker   bool   `json:"m"`
	Time      int64  `json:"T"`
}

type depthMsg struct {
	Event     string     `json:"e"`
	EventTime int64      `json:"E"`
	Bids      [][]string `json:"b"`
	Asks      [][]string `json:"a"`
}

type markPriceMsg struct {
	Event       string `json:"e"`
	EventTime   int64  `json:"E"`
	Symbol      string `json:"s"`
	MarkPrice   string `json:"p"`
	FundingRate string `json:"r"`
	Time        int64  `json:"T"`
}

type klineMsg struct {
	Event     string `json:"e"`
	EventTime int64  `json:"E"`
	Kline     kline  `json:"k"`
}

type kline struct {
	StartTime int64  `json:"t"`
	Open      string `json:"o"`
	High      string `json:"h"`
	Low       string `json:"l"`
	Close     string `json:"c"`
	Volume    string `json:"v"`
	IsClosed  bool   `json:"x"`
}

func (f *BinancePublicFeed) handleMessage(raw []byte) {
	f.msgOnce.Do(func() {
		fmt.Fprintf(os.Stderr, "TradeFox: receiving live data\n")
	})

	var wrapper combinedStreamMsg
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return
	}

	stream := wrapper.Stream
	switch {
	case strings.Contains(stream, "@aggTrade"):
		f.handleAggTrade(wrapper.Data)
	case strings.Contains(stream, "@depth"):
		f.handleDepth(wrapper.Data)
	case strings.Contains(stream, "@markPrice"):
		f.handleMarkPrice(wrapper.Data)
	case strings.Contains(stream, "@kline"):
		f.handleKline(wrapper.Data)
	}
}

func (f *BinancePublicFeed) handleAggTrade(data json.RawMessage) {
	f.mu.RLock()
	cb := f.callbacks.onTrade
	f.mu.RUnlock()
	if cb == nil {
		return
	}

	var msg aggTradeMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	price, _ := strconv.ParseFloat(msg.Price, 64)
	qty, _ := strconv.ParseFloat(msg.Quantity, 64)

	side := "BUY"
	if msg.IsMaker {
		// m=true means buyer is maker → aggressor is SELL
		side = "SELL"
	}

	cb(TradeEvent{
		Symbol: strings.ToUpper(msg.Symbol),
		Price:  price,
		Size:   qty,
		Side:   side,
		Time:   time.UnixMilli(msg.Time),
	})
}

func (f *BinancePublicFeed) handleDepth(data json.RawMessage) {
	f.mu.RLock()
	cb := f.callbacks.onBook
	f.mu.RUnlock()
	if cb == nil {
		return
	}

	var msg depthMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	bids := make([]OrderBookLevel, 0, len(msg.Bids))
	for _, b := range msg.Bids {
		if len(b) < 2 {
			continue
		}
		price, _ := strconv.ParseFloat(b[0], 64)
		qty, _ := strconv.ParseFloat(b[1], 64)
		bids = append(bids, OrderBookLevel{Price: price, Qty: qty, IsBid: true})
	}

	asks := make([]OrderBookLevel, 0, len(msg.Asks))
	for _, a := range msg.Asks {
		if len(a) < 2 {
			continue
		}
		price, _ := strconv.ParseFloat(a[0], 64)
		qty, _ := strconv.ParseFloat(a[1], 64)
		asks = append(asks, OrderBookLevel{Price: price, Qty: qty, IsBid: false})
	}

	cb(bids, asks)
}

func (f *BinancePublicFeed) handleMarkPrice(data json.RawMessage) {
	f.mu.RLock()
	cb := f.callbacks.onTicker
	f.mu.RUnlock()
	if cb == nil {
		return
	}

	var msg markPriceMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	price, _ := strconv.ParseFloat(msg.MarkPrice, 64)
	fundingRate, _ := strconv.ParseFloat(msg.FundingRate, 64)

	cb(TickerUpdate{
		Symbol:      strings.ToUpper(msg.Symbol),
		Price:       price,
		FundingRate: fundingRate,
	})
}

func (f *BinancePublicFeed) handleKline(data json.RawMessage) {
	f.mu.RLock()
	cb := f.callbacks.onCandle
	f.mu.RUnlock()
	if cb == nil {
		return
	}

	var msg klineMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	open, _ := strconv.ParseFloat(msg.Kline.Open, 64)
	high, _ := strconv.ParseFloat(msg.Kline.High, 64)
	low, _ := strconv.ParseFloat(msg.Kline.Low, 64)
	cl, _ := strconv.ParseFloat(msg.Kline.Close, 64)
	vol, _ := strconv.ParseFloat(msg.Kline.Volume, 64)

	cb(Candle{
		Time:   msg.Kline.StartTime / 1000,
		Open:   open,
		High:   high,
		Low:    low,
		Close:  cl,
		Volume: vol,
	})
}

// reconnect attempts to re-establish the WebSocket connection with exponential backoff.
func (f *BinancePublicFeed) reconnect() {
	f.mu.Lock()
	if f.conn != nil {
		_ = f.conn.Close()
		f.conn = nil
	}
	attempt := f.reconnectAttempt
	f.reconnectAttempt++
	f.mu.Unlock()

	// Exponential backoff: 1s, 2s, 4s, 8s... capped at 30s
	backoff := 1 << uint(attempt)
	if backoff > 30 {
		backoff = 30
	}
	delay := time.Duration(backoff) * time.Second

	fmt.Fprintf(os.Stderr, "TradeFox: reconnecting...\n")

	select {
	case <-f.ctx.Done():
		return
	case <-time.After(delay):
	}

	if err := f.dial(); err != nil {
		// Will retry on next readLoop iteration
		return
	}
}

// Close shuts down the feed.
func (f *BinancePublicFeed) Close() {
	if f.cancel != nil {
		f.cancel()
	}
	f.mu.Lock()
	if f.conn != nil {
		_ = f.conn.Close()
		f.conn = nil
	}
	f.mu.Unlock()
}

// Symbol returns the uppercase symbol this feed is connected to.
func (f *BinancePublicFeed) Symbol() string {
	return strings.ToUpper(f.symbol)
}
