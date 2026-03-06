package kraken

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
	connector "github.com/ashark-ai-05/tradefox/internal/connector"
	"github.com/ashark-ai-05/tradefox/internal/core/enums"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

const defaultWSURL = "wss://ws.kraken.com/v2"

// Connector is the Kraken exchange connector for VisualHFT.
type Connector struct {
	*connector.BaseConnector

	settings Settings
	wsURL    string
	logger   *slog.Logger
	bus      *eventbus.Bus

	conn   *websocket.Conn
	cancel context.CancelFunc
	done   chan struct{}

	// orderBooks stores per-symbol order books.
	orderBooks sync.Map // map[string]*models.OrderBook
}

// New creates a new Kraken Connector. The connector is in the Loaded state
// and must be started with StartAsync.
func New(settings Settings, bus *eventbus.Bus, logger *slog.Logger) *Connector {
	if logger == nil {
		logger = slog.Default()
	}
	if settings.DepthLevels <= 0 {
		settings.DepthLevels = 25
	}

	base := connector.NewBaseConnector(connector.BaseConnectorConfig{
		Name:         "Kraken",
		Version:      "1.0.0",
		Description:  "Kraken WebSocket v2 connector",
		Author:       "VisualHFT",
		ProviderID:   settings.ProviderID,
		ProviderName: settings.ProviderName,
		Bus:          bus,
		Logger:       logger,
	})

	c := &Connector{
		BaseConnector: base,
		settings:      settings,
		wsURL:         defaultWSURL,
		logger:        logger,
		bus:           bus,
	}

	// Kraken uses symbols like "BTC/USD" directly; parse for normalization.
	base.ParseSymbols(strings.Join(settings.Symbols, ","))
	base.SetReconnectionAction(c.connect)

	return c
}

// SetWSURL overrides the default WebSocket URL. Useful for testing.
func (c *Connector) SetWSURL(url string) {
	c.wsURL = url
}

// StartAsync connects to Kraken, subscribes to channels, and starts
// the read loop.
func (c *Connector) StartAsync(ctx context.Context) error {
	if err := c.BaseConnector.StartAsync(ctx); err != nil {
		return err
	}

	if err := c.connect(ctx); err != nil {
		c.SetStatus(enums.PluginStoppedFailed)
		return err
	}

	c.SetStatus(enums.PluginStarted)
	c.PublishProvider(c.GetProviderModel(enums.SessionConnected))

	readCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.done = make(chan struct{})

	go c.readLoop(readCtx)

	return nil
}

// StopAsync gracefully shuts down the connector.
func (c *Connector) StopAsync(ctx context.Context) error {
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}

	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}

	if c.done != nil {
		<-c.done
		c.done = nil
	}

	return c.BaseConnector.StopAsync(ctx)
}

// connect dials the Kraken WebSocket and sends subscription messages.
func (c *Connector) connect(ctx context.Context) error {
	c.logger.Info("kraken connecting", slog.String("url", c.wsURL))

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, c.wsURL, nil)
	if err != nil {
		return fmt.Errorf("kraken dial: %w", err)
	}

	c.conn = conn

	// Collect exchange symbols for subscription.
	exchangeSymbols := c.GetAllExchangeSymbols()

	// Initialize order books.
	for _, sym := range exchangeSymbols {
		normalized := c.GetNormalizedSymbol(sym)
		ob := models.NewOrderBook(normalized, 2, c.settings.DepthLevels)
		ob.ProviderID = c.settings.ProviderID
		ob.ProviderName = c.settings.ProviderName
		ob.ProviderStatus = enums.SessionConnected
		c.orderBooks.Store(normalized, ob)
	}

	// Subscribe to book channel.
	bookSub := map[string]interface{}{
		"method": "subscribe",
		"params": map[string]interface{}{
			"channel": "book",
			"symbol":  exchangeSymbols,
			"depth":   c.settings.DepthLevels,
		},
	}
	if err := conn.WriteJSON(bookSub); err != nil {
		return fmt.Errorf("kraken subscribe book: %w", err)
	}

	// Subscribe to trade channel.
	tradeSub := map[string]interface{}{
		"method": "subscribe",
		"params": map[string]interface{}{
			"channel": "trade",
			"symbol":  exchangeSymbols,
		},
	}
	if err := conn.WriteJSON(tradeSub); err != nil {
		return fmt.Errorf("kraken subscribe trade: %w", err)
	}

	c.logger.Info("kraken connected and subscribed")
	return nil
}

// readLoop reads messages from the WebSocket and dispatches them.
func (c *Connector) readLoop(ctx context.Context) {
	defer close(c.done)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if c.conn == nil {
			return
		}

		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
			}
			c.logger.Warn("kraken read error", slog.Any("error", err))
			c.HandleConnectionLost(ctx, "read error", err)
			return
		}

		c.handleMessage(msg)
	}
}

// krakenMsg is the top-level envelope for Kraken WS v2 messages.
type krakenMsg struct {
	Channel string          `json:"channel"`
	Type    string          `json:"type"`
	Data    json.RawMessage `json:"data"`
}

// krakenBookEntry is a single price level in a Kraken book message.
type krakenBookEntry struct {
	Price float64 `json:"price"`
	Qty   float64 `json:"qty"`
}

// krakenBookData is the data payload for a Kraken book message.
type krakenBookData struct {
	Symbol   string            `json:"symbol"`
	Bids     []krakenBookEntry `json:"bids"`
	Asks     []krakenBookEntry `json:"asks"`
	Checksum int64             `json:"checksum"`
}

// krakenTradeData is a single trade in a Kraken trade message.
type krakenTradeData struct {
	Symbol    string  `json:"symbol"`
	Price     float64 `json:"price"`
	Qty       float64 `json:"qty"`
	Side      string  `json:"side"`
	Timestamp string  `json:"timestamp"`
}

// handleMessage parses a raw Kraken v2 message and dispatches it.
func (c *Connector) handleMessage(raw []byte) {
	var msg krakenMsg
	if err := json.Unmarshal(raw, &msg); err != nil {
		c.logger.Warn("kraken: failed to unmarshal message", slog.Any("error", err))
		return
	}

	switch msg.Channel {
	case "book":
		c.handleBook(msg)
	case "trade":
		c.handleTrade(msg)
	case "heartbeat", "status":
		// Ignored.
	}
}

// handleBook processes order book snapshots and updates.
func (c *Connector) handleBook(msg krakenMsg) {
	var entries []krakenBookData
	if err := json.Unmarshal(msg.Data, &entries); err != nil {
		c.logger.Warn("kraken: failed to unmarshal book data", slog.Any("error", err))
		return
	}

	for _, entry := range entries {
		// Kraken uses the symbol directly as the exchange symbol.
		normalized := c.GetNormalizedSymbol(entry.Symbol)

		obVal, ok := c.orderBooks.Load(normalized)
		if !ok {
			continue
		}
		ob := obVal.(*models.OrderBook)

		switch msg.Type {
		case "snapshot":
			c.processBookSnapshot(ob, entry, normalized)
		case "update":
			c.processBookUpdate(ob, entry, normalized)
		}
	}
}

// processBookSnapshot loads a full order book snapshot from Kraken.
func (c *Connector) processBookSnapshot(ob *models.OrderBook, data krakenBookData, normalized string) {
	now := time.Now()
	bids := make([]models.BookItem, 0, len(data.Bids))
	asks := make([]models.BookItem, 0, len(data.Asks))

	for _, b := range data.Bids {
		bids = append(bids, models.BookItem{
			Symbol:         normalized,
			ProviderID:     c.settings.ProviderID,
			IsBid:          true,
			Price:          floatPtr(b.Price),
			Size:           floatPtr(b.Qty),
			LocalTimestamp:  now,
			ServerTimestamp: now,
		})
	}

	for _, a := range data.Asks {
		asks = append(asks, models.BookItem{
			Symbol:         normalized,
			ProviderID:     c.settings.ProviderID,
			IsBid:          false,
			Price:          floatPtr(a.Price),
			Size:           floatPtr(a.Qty),
			LocalTimestamp:  now,
			ServerTimestamp: now,
		})
	}

	ob.LoadData(asks, bids)
	c.PublishOrderBook(ob)
}

// processBookUpdate applies incremental order book updates from Kraken.
func (c *Connector) processBookUpdate(ob *models.OrderBook, data krakenBookData, normalized string) {
	now := time.Now()

	for _, b := range data.Bids {
		isBidPtr := boolPtr(true)
		if b.Qty == 0 {
			ob.DeleteLevel(models.DeltaBookItem{
				IsBid:          isBidPtr,
				Price:          floatPtr(b.Price),
				Size:           floatPtr(0),
				LocalTimestamp:  now,
				ServerTimestamp: now,
			})
		} else {
			ob.AddOrUpdateLevel(models.DeltaBookItem{
				IsBid:          isBidPtr,
				Price:          floatPtr(b.Price),
				Size:           floatPtr(b.Qty),
				LocalTimestamp:  now,
				ServerTimestamp: now,
			})
		}
	}

	for _, a := range data.Asks {
		isBidPtr := boolPtr(false)
		if a.Qty == 0 {
			ob.DeleteLevel(models.DeltaBookItem{
				IsBid:          isBidPtr,
				Price:          floatPtr(a.Price),
				Size:           floatPtr(0),
				LocalTimestamp:  now,
				ServerTimestamp: now,
			})
		} else {
			ob.AddOrUpdateLevel(models.DeltaBookItem{
				IsBid:          isBidPtr,
				Price:          floatPtr(a.Price),
				Size:           floatPtr(a.Qty),
				LocalTimestamp:  now,
				ServerTimestamp: now,
			})
		}
	}

	c.PublishOrderBook(ob)
}

// handleTrade processes trade messages from Kraken.
func (c *Connector) handleTrade(msg krakenMsg) {
	// Only process updates, not snapshots.
	if msg.Type != "update" {
		return
	}

	var trades []krakenTradeData
	if err := json.Unmarshal(msg.Data, &trades); err != nil {
		c.logger.Warn("kraken: failed to unmarshal trade data", slog.Any("error", err))
		return
	}

	for _, t := range trades {
		normalized := c.GetNormalizedSymbol(t.Symbol)
		isBuy := t.Side == "buy"

		ts, err := time.Parse(time.RFC3339Nano, t.Timestamp)
		if err != nil {
			ts = time.Now()
		}

		trade := models.Trade{
			ProviderID:   c.settings.ProviderID,
			ProviderName: c.settings.ProviderName,
			Symbol:       normalized,
			Price:        decimal.NewFromFloat(t.Price),
			Size:         decimal.NewFromFloat(t.Qty),
			Timestamp:    ts,
			IsBuy:        boolPtr(isBuy),
		}

		c.PublishTrade(trade)
	}
}

func floatPtr(v float64) *float64 { return &v }
func boolPtr(v bool) *bool        { return &v }
