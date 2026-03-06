package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"

	"github.com/ashark-ai-05/tradefox/internal/config"
	connector "github.com/ashark-ai-05/tradefox/internal/connector"
	"github.com/ashark-ai-05/tradefox/internal/core/enums"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

// WebSocket endpoint templates.
const (
	wsGlobalBase = "wss://stream.binance.com:9443"
	wsUSBase     = "wss://stream.binance.us:9443"

	restGlobalBase = "https://api.binance.com"
	restUSBase     = "https://api.binance.us"
)

// symbolBook tracks per-symbol order book state including the last update ID
// used for the snapshot-then-delta synchronisation protocol.
type symbolBook struct {
	mu           sync.Mutex
	ob           *models.OrderBook
	lastUpdateID int64
	snapshotDone bool
	buffer       []depthMsg // buffered deltas received before the snapshot
}

// BinanceConnector streams order book and trade data from Binance via
// WebSocket, using REST snapshots for initial order book state.
type BinanceConnector struct {
	*connector.BaseConnector

	settings Settings
	logger   *slog.Logger
	bus      *eventbus.Bus

	books sync.Map // map[string]*symbolBook

	wsConn   *websocket.Conn
	wsMu     sync.Mutex
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	// Overridable for testing.
	wsURL      string
	restURL    string
	httpClient *http.Client
}

// New creates a BinanceConnector. The caller should invoke StartAsync to begin
// streaming.
func New(bus *eventbus.Bus, settings *config.Manager, logger *slog.Logger) *BinanceConnector {
	if logger == nil {
		logger = slog.Default()
	}

	s := DefaultSettings()

	bc := &BinanceConnector{
		BaseConnector: connector.NewBaseConnector(connector.BaseConnectorConfig{
			Name:         "Binance",
			Version:      "1.0.0",
			Description:  "Binance market data connector using WebSocket streams",
			Author:       "VisualHFT",
			ProviderID:   s.ProviderID,
			ProviderName: s.ProviderName,
			Bus:          bus,
			Settings:     settings,
			Logger:       logger,
		}),
		settings:   s,
		logger:     logger,
		bus:        bus,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}

	bc.SetReconnectionAction(bc.connect)
	return bc
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// StartAsync connects to Binance and begins streaming market data.
func (c *BinanceConnector) StartAsync(ctx context.Context) error {
	if err := c.BaseConnector.StartAsync(ctx); err != nil {
		return err
	}

	// Try to load user settings; fall back to defaults on error.
	var loaded Settings
	if err := c.LoadFromUserSettings(&loaded); err == nil {
		c.settings = loaded
	}

	// Apply settings to BaseConnector metadata.
	c.applySettings()

	return c.connect(ctx)
}

// StopAsync gracefully shuts down the connector.
func (c *BinanceConnector) StopAsync(ctx context.Context) error {
	if c.cancel != nil {
		c.cancel()
	}

	c.wsMu.Lock()
	if c.wsConn != nil {
		_ = c.wsConn.Close()
		c.wsConn = nil
	}
	c.wsMu.Unlock()

	c.wg.Wait()
	return c.BaseConnector.StopAsync(ctx)
}

// ---------------------------------------------------------------------------
// Connection
// ---------------------------------------------------------------------------

func (c *BinanceConnector) connect(ctx context.Context) error {
	// Parse symbols.
	symStr := strings.Join(c.settings.Symbols, ",")
	c.ParseSymbols(symStr)
	exchangeSymbols := c.GetAllExchangeSymbols()

	if len(exchangeSymbols) == 0 {
		return fmt.Errorf("binance: no symbols configured")
	}

	// Build the combined stream URL.
	wsBase := c.wsBaseURL()
	streams := c.buildStreams(exchangeSymbols)
	wsURL := fmt.Sprintf("%s/stream?streams=%s", wsBase, strings.Join(streams, "/"))

	// Allow test override.
	if c.wsURL != "" {
		wsURL = c.wsURL
	}

	// Initialise per-symbol books.
	for _, sym := range exchangeSymbols {
		lowerSym := strings.ToLower(sym)
		normalised := c.GetNormalizedSymbol(sym)
		ob := models.NewOrderBook(normalised, 8, c.settings.DepthLevels)
		ob.ProviderID = c.settings.ProviderID
		ob.ProviderName = c.settings.ProviderName
		ob.ProviderStatus = enums.SessionConnected
		c.books.Store(lowerSym, &symbolBook{
			ob:     ob,
			buffer: make([]depthMsg, 0, 64),
		})
	}

	// Connect WebSocket.
	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("binance: websocket dial: %w", err)
	}

	c.wsMu.Lock()
	c.wsConn = conn
	c.wsMu.Unlock()

	innerCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	c.SetStatus(enums.PluginStarted)
	c.PublishProvider(c.GetProviderModel(enums.SessionConnected))

	// Fetch REST snapshots in background.
	for _, sym := range exchangeSymbols {
		sym := sym
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			c.fetchSnapshot(innerCtx, sym)
		}()
	}

	// Start consumer goroutine.
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.readLoop(innerCtx)
	}()

	return nil
}

func (c *BinanceConnector) applySettings() {
	// Re-parse symbols in case settings changed.
	symStr := strings.Join(c.settings.Symbols, ",")
	c.ParseSymbols(symStr)
}

func (c *BinanceConnector) wsBaseURL() string {
	if c.wsURL != "" {
		// Already overridden (tests).
		return c.wsURL
	}
	if c.settings.IsNonUS {
		return wsGlobalBase
	}
	return wsUSBase
}

func (c *BinanceConnector) restBaseURL() string {
	if c.restURL != "" {
		return c.restURL
	}
	if c.settings.IsNonUS {
		return restGlobalBase
	}
	return restUSBase
}

func (c *BinanceConnector) buildStreams(symbols []string) []string {
	streams := make([]string, 0, len(symbols)*2)
	interval := c.settings.UpdateIntervalMs
	if interval <= 0 {
		interval = 100
	}
	for _, sym := range symbols {
		lower := strings.ToLower(sym)
		streams = append(streams,
			fmt.Sprintf("%s@depth@%dms", lower, interval),
			fmt.Sprintf("%s@trade", lower),
		)
	}
	return streams
}

// ---------------------------------------------------------------------------
// REST snapshot
// ---------------------------------------------------------------------------

// snapshotResponse represents the JSON response from the REST depth endpoint.
type snapshotResponse struct {
	LastUpdateID int64      `json:"lastUpdateId"`
	Bids         [][]string `json:"bids"`
	Asks         [][]string `json:"asks"`
}

func (c *BinanceConnector) fetchSnapshot(ctx context.Context, exchangeSymbol string) {
	upperSym := strings.ToUpper(exchangeSymbol)
	url := fmt.Sprintf("%s/api/v3/depth?symbol=%s&limit=%d",
		c.restBaseURL(), upperSym, c.settings.DepthLevels)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		c.logger.Error("binance: snapshot request build failed",
			slog.String("symbol", exchangeSymbol), slog.Any("error", err))
		return
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("binance: snapshot request failed",
			slog.String("symbol", exchangeSymbol), slog.Any("error", err))
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Error("binance: snapshot body read failed",
			slog.String("symbol", exchangeSymbol), slog.Any("error", err))
		return
	}

	var snap snapshotResponse
	if err := json.Unmarshal(body, &snap); err != nil {
		c.logger.Error("binance: snapshot parse failed",
			slog.String("symbol", exchangeSymbol), slog.Any("error", err))
		return
	}

	lowerSym := strings.ToLower(exchangeSymbol)
	val, ok := c.books.Load(lowerSym)
	if !ok {
		return
	}
	sb := val.(*symbolBook)

	sb.mu.Lock()
	defer sb.mu.Unlock()

	// Load snapshot into order book.
	bids := parseLevels(snap.Bids, true)
	asks := parseLevels(snap.Asks, false)
	sb.ob.LoadData(asks, bids)
	sb.lastUpdateID = snap.LastUpdateID
	sb.snapshotDone = true

	// Apply any buffered deltas.
	for _, d := range sb.buffer {
		c.applyDepthDelta(sb, d)
	}
	sb.buffer = sb.buffer[:0]

	c.PublishOrderBook(sb.ob)
}

// ---------------------------------------------------------------------------
// WebSocket message types
// ---------------------------------------------------------------------------

// combinedMsg wraps the Binance combined stream message format.
type combinedMsg struct {
	Stream string          `json:"stream"`
	Data   json.RawMessage `json:"data"`
}

// depthMsg represents a depth update message from Binance.
type depthMsg struct {
	EventType    string     `json:"e"`
	EventTime    int64      `json:"E"`
	Symbol       string     `json:"s"`
	FirstUpdateID int64     `json:"U"`
	FinalUpdateID int64     `json:"u"`
	Bids         [][]string `json:"b"`
	Asks         [][]string `json:"a"`
}

// tradeMsg represents a trade message from Binance.
type tradeMsg struct {
	EventType  string `json:"e"`
	EventTime  int64  `json:"E"`
	Symbol     string `json:"s"`
	Price      string `json:"p"`
	Quantity   string `json:"q"`
	TradeTime  int64  `json:"T"`
	IsBuyerMaker bool `json:"m"`
}

// ---------------------------------------------------------------------------
// Read loop
// ---------------------------------------------------------------------------

func (c *BinanceConnector) readLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		c.wsMu.Lock()
		conn := c.wsConn
		c.wsMu.Unlock()

		if conn == nil {
			return
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return // context cancelled, normal shutdown
			}
			c.logger.Warn("binance: websocket read error", slog.Any("error", err))
			c.HandleConnectionLost(ctx, "websocket read error", err)
			return
		}

		c.dispatch(message)
	}
}

func (c *BinanceConnector) dispatch(raw []byte) {
	var combined combinedMsg
	if err := json.Unmarshal(raw, &combined); err != nil {
		// Try parsing as a direct message (non-combined stream).
		c.dispatchDirect(raw)
		return
	}

	// If the stream field is empty this is not a combined-stream message;
	// fall through to direct dispatch.
	if combined.Stream == "" || len(combined.Data) == 0 {
		c.dispatchDirect(raw)
		return
	}

	// Determine stream type from the stream name.
	stream := combined.Stream
	switch {
	case strings.Contains(stream, "@depth"):
		var d depthMsg
		if err := json.Unmarshal(combined.Data, &d); err != nil {
			c.logger.Warn("binance: depth parse error", slog.Any("error", err))
			return
		}
		c.handleDepth(d)

	case strings.Contains(stream, "@trade"):
		var t tradeMsg
		if err := json.Unmarshal(combined.Data, &t); err != nil {
			c.logger.Warn("binance: trade parse error", slog.Any("error", err))
			return
		}
		c.handleTrade(t)
	}
}

// dispatchDirect handles messages not wrapped in the combined stream envelope.
func (c *BinanceConnector) dispatchDirect(raw []byte) {
	// Peek at the event type using a map to avoid Go's case-insensitive JSON
	// key matching colliding "e" (event type) with "E" (event time).
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return
	}

	var eventType string
	if v, ok := m["e"]; ok {
		if err := json.Unmarshal(v, &eventType); err != nil {
			return
		}
	}

	switch eventType {
	case "depthUpdate":
		var d depthMsg
		if err := json.Unmarshal(raw, &d); err == nil {
			c.handleDepth(d)
		}
	case "trade":
		var t tradeMsg
		if err := json.Unmarshal(raw, &t); err == nil {
			c.handleTrade(t)
		}
	}
}

// ---------------------------------------------------------------------------
// Depth handling
// ---------------------------------------------------------------------------

func (c *BinanceConnector) handleDepth(d depthMsg) {
	lowerSym := strings.ToLower(d.Symbol)
	val, ok := c.books.Load(lowerSym)
	if !ok {
		return
	}
	sb := val.(*symbolBook)

	sb.mu.Lock()
	defer sb.mu.Unlock()

	if !sb.snapshotDone {
		// Buffer until snapshot is applied.
		sb.buffer = append(sb.buffer, d)
		return
	}

	c.applyDepthDelta(sb, d)
	c.PublishOrderBook(sb.ob)
}

// applyDepthDelta applies a single depth delta to the symbol book.
// Caller must hold sb.mu.
func (c *BinanceConnector) applyDepthDelta(sb *symbolBook, d depthMsg) {
	// Per Binance docs: drop events where u < lastUpdateID+1
	// Accept events where U <= lastUpdateID+1 && u >= lastUpdateID+1
	if d.FinalUpdateID < sb.lastUpdateID+1 {
		return // stale
	}

	now := time.Now()

	// Apply bid deltas.
	for _, entry := range d.Bids {
		if len(entry) < 2 {
			continue
		}
		price, err := strconv.ParseFloat(entry[0], 64)
		if err != nil {
			continue
		}
		size, err := strconv.ParseFloat(entry[1], 64)
		if err != nil {
			continue
		}

		isBid := true
		delta := models.DeltaBookItem{
			IsBid:          &isBid,
			Price:          &price,
			Size:           &size,
			LocalTimestamp:  now,
			ServerTimestamp: time.UnixMilli(d.EventTime),
		}

		if size == 0 {
			sb.ob.DeleteLevel(delta)
		} else {
			sb.ob.AddOrUpdateLevel(delta)
		}
	}

	// Apply ask deltas.
	for _, entry := range d.Asks {
		if len(entry) < 2 {
			continue
		}
		price, err := strconv.ParseFloat(entry[0], 64)
		if err != nil {
			continue
		}
		size, err := strconv.ParseFloat(entry[1], 64)
		if err != nil {
			continue
		}

		isBid := false
		delta := models.DeltaBookItem{
			IsBid:          &isBid,
			Price:          &price,
			Size:           &size,
			LocalTimestamp:  now,
			ServerTimestamp: time.UnixMilli(d.EventTime),
		}

		if size == 0 {
			sb.ob.DeleteLevel(delta)
		} else {
			sb.ob.AddOrUpdateLevel(delta)
		}
	}

	sb.lastUpdateID = d.FinalUpdateID
}

// ---------------------------------------------------------------------------
// Trade handling
// ---------------------------------------------------------------------------

func (c *BinanceConnector) handleTrade(t tradeMsg) {
	price, err := decimal.NewFromString(t.Price)
	if err != nil {
		c.logger.Warn("binance: trade price parse error", slog.Any("error", err))
		return
	}
	size, err := decimal.NewFromString(t.Quantity)
	if err != nil {
		c.logger.Warn("binance: trade size parse error", slog.Any("error", err))
		return
	}

	lowerSym := strings.ToLower(t.Symbol)
	normalised := c.GetNormalizedSymbol(strings.ToUpper(t.Symbol))

	// m=true means buyer is market maker, so the trade is a sell.
	isBuy := !t.IsBuyerMaker

	// Get mid price from the order book if available.
	var midPrice float64
	if val, ok := c.books.Load(lowerSym); ok {
		sb := val.(*symbolBook)
		sb.mu.Lock()
		midPrice = sb.ob.MidPrice()
		sb.mu.Unlock()
	}

	trade := models.Trade{
		ProviderID:     c.settings.ProviderID,
		ProviderName:   c.settings.ProviderName,
		Symbol:         normalised,
		Price:          price,
		Size:           size,
		Timestamp:      time.UnixMilli(t.TradeTime),
		IsBuy:          &isBuy,
		MarketMidPrice: midPrice,
	}

	c.PublishTrade(trade)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// parseLevels converts Binance's [["price","qty"], ...] format into BookItem
// slices.
func parseLevels(entries [][]string, isBid bool) []models.BookItem {
	items := make([]models.BookItem, 0, len(entries))
	now := time.Now()
	for _, entry := range entries {
		if len(entry) < 2 {
			continue
		}
		price, err := strconv.ParseFloat(entry[0], 64)
		if err != nil {
			continue
		}
		size, err := strconv.ParseFloat(entry[1], 64)
		if err != nil {
			continue
		}
		items = append(items, models.BookItem{
			Price:          &price,
			Size:           &size,
			IsBid:          isBid,
			LocalTimestamp:  now,
			ServerTimestamp: now,
		})
	}
	return items
}
