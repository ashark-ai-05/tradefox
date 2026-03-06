package bitstamp

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
	"github.com/ashark-ai-05/tradefox/internal/connector"
	"github.com/ashark-ai-05/tradefox/internal/core/enums"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

// BitStampConnector connects to the BitStamp exchange via WebSocket for
// real-time order book deltas and trades, and uses the REST API for
// initial order book snapshots.
type BitStampConnector struct {
	*connector.BaseConnector

	settings Settings
	logger   *slog.Logger
	bus      *eventbus.Bus

	conn   *websocket.Conn
	connMu sync.Mutex

	orderBooks   map[string]*models.OrderBook // keyed by normalized symbol
	orderBooksMu sync.RWMutex

	httpClient *http.Client

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates a new BitStampConnector with the given settings, event bus, and
// logger.
func New(settings Settings, bus *eventbus.Bus, logger *slog.Logger) *BitStampConnector {
	if logger == nil {
		logger = slog.Default()
	}
	if settings.DepthLevels <= 0 {
		settings.DepthLevels = 10
	}

	bc := connector.NewBaseConnector(connector.BaseConnectorConfig{
		Name:         "BitStamp",
		Version:      "1.0.0",
		Description:  "BitStamp exchange connector",
		Author:       "VisualHFT",
		ProviderID:   settings.ProviderID,
		ProviderName: settings.ProviderName,
		Bus:          bus,
		Logger:       logger,
	})

	c := &BitStampConnector{
		BaseConnector: bc,
		settings:      settings,
		logger:        logger,
		bus:           bus,
		orderBooks:    make(map[string]*models.OrderBook),
		httpClient:    &http.Client{Timeout: 10 * time.Second},
	}

	// Parse symbols from the settings list. Each entry has the form
	// "btcusd(BTC/USD)" where btcusd is the exchange symbol and BTC/USD is the
	// normalized symbol.
	for _, sym := range settings.Symbols {
		bc.ParseSymbols(sym)
	}

	bc.SetReconnectionAction(c.connect)
	return c
}

// StartAsync initiates the WebSocket connection and begins reading messages.
func (c *BitStampConnector) StartAsync(ctx context.Context) error {
	if err := c.BaseConnector.StartAsync(ctx); err != nil {
		return err
	}

	return c.connect(ctx)
}

// StopAsync gracefully shuts down the connector.
func (c *BitStampConnector) StopAsync(ctx context.Context) error {
	if c.cancel != nil {
		c.cancel()
	}

	c.connMu.Lock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.connMu.Unlock()

	c.wg.Wait()
	return c.BaseConnector.StopAsync(ctx)
}

// connect establishes the WebSocket connection, subscribes to channels,
// fetches initial order book snapshots, and starts the read loop.
func (c *BitStampConnector) connect(ctx context.Context) error {
	childCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	dialer := websocket.DefaultDialer
	ws, _, err := dialer.DialContext(childCtx, c.settings.WebSocketHostName, nil)
	if err != nil {
		cancel()
		return fmt.Errorf("bitstamp: websocket dial: %w", err)
	}

	c.connMu.Lock()
	c.conn = ws
	c.connMu.Unlock()

	// Subscribe to channels for each exchange symbol.
	for _, exchangeSym := range c.BaseConnector.GetAllExchangeSymbols() {
		sym := strings.ToLower(exchangeSym)

		// Subscribe to order book diff channel.
		if err := c.subscribe(ws, "diff_order_book_"+sym); err != nil {
			cancel()
			return fmt.Errorf("bitstamp: subscribe order book %s: %w", sym, err)
		}

		// 1-second delay between subscriptions as per BitStamp guidelines.
		select {
		case <-childCtx.Done():
			return childCtx.Err()
		case <-time.After(1 * time.Second):
		}

		// Subscribe to live trades channel.
		if err := c.subscribe(ws, "live_trades_"+sym); err != nil {
			cancel()
			return fmt.Errorf("bitstamp: subscribe trades %s: %w", sym, err)
		}

		// Fetch initial REST snapshot.
		if err := c.fetchSnapshot(childCtx, exchangeSym); err != nil {
			c.logger.Warn("bitstamp: failed to fetch snapshot",
				slog.String("symbol", exchangeSym),
				slog.Any("error", err),
			)
		}
	}

	c.SetStatus(enums.PluginStarted)
	c.PublishProvider(c.GetProviderModel(enums.SessionConnected))

	// Start read loop.
	c.wg.Add(1)
	go c.readLoop(childCtx)

	return nil
}

// subscribe sends a subscription message for the given channel.
func (c *BitStampConnector) subscribe(ws *websocket.Conn, channel string) error {
	msg := map[string]interface{}{
		"event": "bts:subscribe",
		"data": map[string]string{
			"channel": channel,
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return ws.WriteMessage(websocket.TextMessage, data)
}

// ---------------------------------------------------------------------------
// REST snapshot
// ---------------------------------------------------------------------------

// snapshotResponse models the BitStamp REST order book response.
type snapshotResponse struct {
	Timestamp string     `json:"timestamp"`
	Bids      [][]string `json:"bids"` // [["price", "qty"], ...]
	Asks      [][]string `json:"asks"` // [["price", "qty"], ...]
}

// fetchSnapshot retrieves the current order book from the REST API and loads
// it into the local order book.
func (c *BitStampConnector) fetchSnapshot(ctx context.Context, exchangeSym string) error {
	sym := strings.ToLower(exchangeSym)
	url := strings.TrimRight(c.settings.HostName, "/") + "/order_book/" + sym + "/?group=1"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("bitstamp: create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("bitstamp: http get: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("bitstamp: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bitstamp: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var snap snapshotResponse
	if err := json.Unmarshal(body, &snap); err != nil {
		return fmt.Errorf("bitstamp: unmarshal snapshot: %w", err)
	}

	normalizedSym := c.BaseConnector.GetNormalizedSymbol(exchangeSym)
	ob := c.getOrCreateOrderBook(normalizedSym)

	bids := parseLevels(snap.Bids, true)
	asks := parseLevels(snap.Asks, false)

	ob.LoadData(asks, bids)

	// Detect decimal places from the snapshot data.
	prices := make([]float64, 0, len(snap.Bids)+len(snap.Asks))
	for _, b := range bids {
		if b.Price != nil {
			prices = append(prices, *b.Price)
		}
	}
	for _, a := range asks {
		if a.Price != nil {
			prices = append(prices, *a.Price)
		}
	}
	ob.PriceDecimalPlaces = connector.RecognizeDecimalPlaces(prices)

	c.PublishOrderBook(ob)
	return nil
}

// parseLevels converts string pairs from the REST response into BookItem slices.
func parseLevels(levels [][]string, isBid bool) []models.BookItem {
	items := make([]models.BookItem, 0, len(levels))
	for _, level := range levels {
		if len(level) < 2 {
			continue
		}
		price, err := strconv.ParseFloat(level[0], 64)
		if err != nil {
			continue
		}
		size, err := strconv.ParseFloat(level[1], 64)
		if err != nil {
			continue
		}
		items = append(items, models.BookItem{
			Price: &price,
			Size:  &size,
			IsBid: isBid,
		})
	}
	return items
}

// ---------------------------------------------------------------------------
// WebSocket message types
// ---------------------------------------------------------------------------

// wsMessage is the top-level WebSocket message envelope.
type wsMessage struct {
	Event   string          `json:"event"`
	Channel string          `json:"channel"`
	Data    json.RawMessage `json:"data"`
}

// orderBookDelta is the data payload for diff_order_book messages.
type orderBookDelta struct {
	Timestamp      string     `json:"timestamp"`
	Microtimestamp string     `json:"microtimestamp"`
	Bids           [][]string `json:"bids"` // [["price", "qty"], ...]
	Asks           [][]string `json:"asks"` // [["price", "qty"], ...]
}

// tradeData is the data payload for live_trades messages.
type tradeData struct {
	ID             int64  `json:"id"`
	Price          string `json:"price"`
	Amount         string `json:"amount"`
	Type           int    `json:"type"` // 0=buy, 1=sell
	Microtimestamp string `json:"microtimestamp"`
}

// ---------------------------------------------------------------------------
// Read loop
// ---------------------------------------------------------------------------

// readLoop reads WebSocket messages until the context is cancelled or an error
// occurs.
func (c *BitStampConnector) readLoop(ctx context.Context) {
	defer c.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		c.connMu.Lock()
		ws := c.conn
		c.connMu.Unlock()

		if ws == nil {
			return
		}

		_, message, err := ws.ReadMessage()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
			}
			c.logger.Warn("bitstamp: read error", slog.Any("error", err))
			c.HandleConnectionLost(ctx, "read error", err)
			return
		}

		c.handleMessage(message)
	}
}

// handleMessage routes a raw WebSocket message to the appropriate handler.
func (c *BitStampConnector) handleMessage(raw []byte) {
	var msg wsMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		c.logger.Warn("bitstamp: unmarshal message", slog.Any("error", err))
		return
	}

	switch {
	case msg.Event == "bts:heartbeat":
		c.PublishProvider(c.GetProviderModel(enums.SessionConnected))

	case strings.HasPrefix(msg.Channel, "diff_order_book_"):
		c.handleOrderBookDelta(msg)

	case strings.HasPrefix(msg.Channel, "live_trades_"):
		c.handleTrade(msg)
	}
}

// extractSymbolFromChannel extracts the exchange symbol from a channel name
// by splitting on "_" and taking the last element.
// e.g. "diff_order_book_btcusd" -> "btcusd"
func extractSymbolFromChannel(channel string) string {
	parts := strings.Split(channel, "_")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

// handleOrderBookDelta processes an order book diff message.
func (c *BitStampConnector) handleOrderBookDelta(msg wsMessage) {
	var delta orderBookDelta
	if err := json.Unmarshal(msg.Data, &delta); err != nil {
		c.logger.Warn("bitstamp: unmarshal delta", slog.Any("error", err))
		return
	}

	exchangeSym := extractSymbolFromChannel(msg.Channel)
	normalizedSym := c.BaseConnector.GetNormalizedSymbol(exchangeSym)
	ob := c.getOrCreateOrderBook(normalizedSym)

	now := time.Now()

	// Parse server timestamp from microtimestamp.
	serverTime := parseMicrotimestamp(delta.Microtimestamp)

	// Apply bid deltas.
	for _, entry := range delta.Bids {
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
		d := models.DeltaBookItem{
			IsBid:          &isBid,
			Price:          &price,
			Size:           &size,
			LocalTimestamp:  now,
			ServerTimestamp: serverTime,
		}

		if size == 0 {
			ob.DeleteLevel(d)
		} else {
			ob.AddOrUpdateLevel(d)
		}
	}

	// Apply ask deltas.
	for _, entry := range delta.Asks {
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
		d := models.DeltaBookItem{
			IsBid:          &isBid,
			Price:          &price,
			Size:           &size,
			LocalTimestamp:  now,
			ServerTimestamp: serverTime,
		}

		if size == 0 {
			ob.DeleteLevel(d)
		} else {
			ob.AddOrUpdateLevel(d)
		}
	}

	c.PublishOrderBook(ob)
}

// handleTrade processes a live trade message.
func (c *BitStampConnector) handleTrade(msg wsMessage) {
	var td tradeData
	if err := json.Unmarshal(msg.Data, &td); err != nil {
		c.logger.Warn("bitstamp: unmarshal trade", slog.Any("error", err))
		return
	}

	exchangeSym := extractSymbolFromChannel(msg.Channel)
	normalizedSym := c.BaseConnector.GetNormalizedSymbol(exchangeSym)

	price, err := decimal.NewFromString(td.Price)
	if err != nil {
		c.logger.Warn("bitstamp: parse trade price", slog.Any("error", err))
		return
	}
	amount, err := decimal.NewFromString(td.Amount)
	if err != nil {
		c.logger.Warn("bitstamp: parse trade amount", slog.Any("error", err))
		return
	}

	isBuy := td.Type == 0
	ts := parseMicrotimestamp(td.Microtimestamp)

	trade := models.Trade{
		ProviderID:   c.settings.ProviderID,
		ProviderName: c.settings.ProviderName,
		Symbol:       normalizedSym,
		Price:        price,
		Size:         amount,
		Timestamp:    ts,
		IsBuy:        &isBuy,
	}

	c.PublishTrade(trade)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// getOrCreateOrderBook returns the order book for the given normalized symbol,
// creating one if it does not yet exist.
func (c *BitStampConnector) getOrCreateOrderBook(normalizedSym string) *models.OrderBook {
	c.orderBooksMu.RLock()
	ob, ok := c.orderBooks[normalizedSym]
	c.orderBooksMu.RUnlock()

	if ok {
		return ob
	}

	c.orderBooksMu.Lock()
	defer c.orderBooksMu.Unlock()

	// Double-check after acquiring write lock.
	if ob, ok = c.orderBooks[normalizedSym]; ok {
		return ob
	}

	ob = models.NewOrderBook(normalizedSym, 2, c.settings.DepthLevels)
	ob.ProviderID = c.settings.ProviderID
	ob.ProviderName = c.settings.ProviderName
	c.orderBooks[normalizedSym] = ob
	return ob
}

// parseMicrotimestamp converts a microsecond timestamp string to time.Time.
// Returns current time if parsing fails.
func parseMicrotimestamp(us string) time.Time {
	if us == "" {
		return time.Now()
	}
	usec, err := strconv.ParseInt(us, 10, 64)
	if err != nil {
		return time.Now()
	}
	sec := usec / 1_000_000
	nsec := (usec % 1_000_000) * 1_000
	return time.Unix(sec, nsec)
}
