package bitfinex

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
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

const defaultWSURL = "wss://api-pub.bitfinex.com/ws/2"

// Connector is the Bitfinex exchange connector for VisualHFT.
type Connector struct {
	*connector.BaseConnector

	settings Settings
	wsURL    string
	logger   *slog.Logger
	bus      *eventbus.Bus

	conn   *websocket.Conn
	cancel context.CancelFunc
	done   chan struct{}

	// channelMap maps Bitfinex channel IDs to their type and symbol.
	channelMap sync.Map // map[int]channelInfo

	// orderBooks stores per-symbol order books.
	orderBooks sync.Map // map[string]*models.OrderBook
}

// channelInfo tracks what kind of data a Bitfinex channel carries.
type channelInfo struct {
	channel string // "book" or "trades"
	symbol  string // exchange symbol, e.g. "tBTCUSD"
}

// New creates a new Bitfinex Connector. The connector is in the Loaded state
// and must be started with StartAsync.
func New(settings Settings, bus *eventbus.Bus, logger *slog.Logger) *Connector {
	if logger == nil {
		logger = slog.Default()
	}
	if settings.DepthLevels <= 0 {
		settings.DepthLevels = 25
	}

	base := connector.NewBaseConnector(connector.BaseConnectorConfig{
		Name:         "Bitfinex",
		Version:      "1.0.0",
		Description:  "Bitfinex WebSocket v2 connector",
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

	// Parse symbols: "BTCUSD(BTC/USD)" -> exchange="BTCUSD", normalized="BTC/USD"
	base.ParseSymbols(strings.Join(settings.Symbols, ","))
	base.SetReconnectionAction(c.connect)

	return c
}

// SetWSURL overrides the default WebSocket URL. Useful for testing.
func (c *Connector) SetWSURL(url string) {
	c.wsURL = url
}

// StartAsync connects to Bitfinex, subscribes to channels, and starts
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

// connect dials the Bitfinex WebSocket and sends subscription messages.
func (c *Connector) connect(ctx context.Context) error {
	c.logger.Info("bitfinex connecting", slog.String("url", c.wsURL))

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, c.wsURL, nil)
	if err != nil {
		return fmt.Errorf("bitfinex dial: %w", err)
	}

	c.conn = conn
	c.channelMap = sync.Map{}

	// Initialize order books and subscribe for each symbol.
	for _, exchangeSym := range c.GetAllExchangeSymbols() {
		normalized := c.GetNormalizedSymbol(exchangeSym)
		ob := models.NewOrderBook(normalized, 2, c.settings.DepthLevels)
		ob.ProviderID = c.settings.ProviderID
		ob.ProviderName = c.settings.ProviderName
		ob.ProviderStatus = enums.SessionConnected
		c.orderBooks.Store(normalized, ob)

		// Bitfinex trading symbols are prefixed with 't'.
		bfxSymbol := "t" + exchangeSym

		// Subscribe to book channel.
		bookSub := map[string]interface{}{
			"event":   "subscribe",
			"channel": "book",
			"symbol":  bfxSymbol,
			"prec":    "P0",
			"freq":    "F0",
			"len":     fmt.Sprintf("%d", c.settings.DepthLevels),
		}
		if err := conn.WriteJSON(bookSub); err != nil {
			return fmt.Errorf("bitfinex subscribe book: %w", err)
		}

		// Subscribe to trades channel.
		tradeSub := map[string]interface{}{
			"event":   "subscribe",
			"channel": "trades",
			"symbol":  bfxSymbol,
		}
		if err := conn.WriteJSON(tradeSub); err != nil {
			return fmt.Errorf("bitfinex subscribe trades: %w", err)
		}
	}

	c.logger.Info("bitfinex connected and subscribed")
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
			c.logger.Warn("bitfinex read error", slog.Any("error", err))
			c.HandleConnectionLost(ctx, "read error", err)
			return
		}

		c.handleMessage(msg)
	}
}

// handleMessage parses a raw Bitfinex message and dispatches it.
func (c *Connector) handleMessage(raw []byte) {
	// Bitfinex sends events as JSON objects and data as JSON arrays.
	if len(raw) == 0 {
		return
	}

	// Event messages (subscribed confirmations, info, etc.) start with '{'.
	if raw[0] == '{' {
		c.handleEvent(raw)
		return
	}

	// Data messages are arrays: [CHANNEL_ID, ...]
	if raw[0] == '[' {
		c.handleData(raw)
		return
	}
}

// handleEvent processes Bitfinex event messages (subscribe confirmations).
func (c *Connector) handleEvent(raw []byte) {
	var evt map[string]interface{}
	if err := json.Unmarshal(raw, &evt); err != nil {
		c.logger.Warn("bitfinex: failed to parse event", slog.Any("error", err))
		return
	}

	eventType, _ := evt["event"].(string)
	if eventType == "subscribed" {
		chanID := int(evt["chanId"].(float64))
		channel, _ := evt["channel"].(string)
		symbol, _ := evt["symbol"].(string)
		c.channelMap.Store(chanID, channelInfo{
			channel: channel,
			symbol:  symbol,
		})
		c.logger.Info("bitfinex subscribed",
			slog.Int("chanId", chanID),
			slog.String("channel", channel),
			slog.String("symbol", symbol),
		)
	}
}

// handleData processes Bitfinex data messages (arrays).
func (c *Connector) handleData(raw []byte) {
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		c.logger.Warn("bitfinex: failed to parse data array", slog.Any("error", err))
		return
	}

	if len(arr) < 2 {
		return
	}

	// First element is the channel ID.
	var chanID int
	if err := json.Unmarshal(arr[0], &chanID); err != nil {
		return
	}

	infoVal, ok := c.channelMap.Load(chanID)
	if !ok {
		return
	}
	info := infoVal.(channelInfo)

	// Check for heartbeat: second element is the string "hb".
	var hbCheck string
	if json.Unmarshal(arr[1], &hbCheck) == nil && hbCheck == "hb" {
		return
	}

	switch info.channel {
	case "book":
		c.handleBookData(info.symbol, arr[1])
	case "trades":
		c.handleTradeData(info.symbol, arr[1:])
	}
}

// handleBookData processes order book snapshots and updates.
func (c *Connector) handleBookData(bfxSymbol string, data json.RawMessage) {
	// Resolve to normalized symbol.
	exchangeSym := strings.TrimPrefix(bfxSymbol, "t")
	normalized := c.GetNormalizedSymbol(exchangeSym)

	obVal, ok := c.orderBooks.Load(normalized)
	if !ok {
		return
	}
	ob := obVal.(*models.OrderBook)

	// Try snapshot first: [[PRICE, COUNT, AMOUNT], ...]
	var snapshot [][]float64
	if err := json.Unmarshal(data, &snapshot); err == nil && len(snapshot) > 0 {
		c.processBookSnapshot(ob, snapshot, normalized)
		return
	}

	// Single update: [PRICE, COUNT, AMOUNT]
	var update []float64
	if err := json.Unmarshal(data, &update); err == nil && len(update) == 3 {
		c.processBookUpdate(ob, update, normalized)
		return
	}
}

// processBookSnapshot loads a full order book snapshot from Bitfinex.
func (c *Connector) processBookSnapshot(ob *models.OrderBook, snapshot [][]float64, normalized string) {
	now := time.Now()
	var bids, asks []models.BookItem

	for _, entry := range snapshot {
		if len(entry) < 3 {
			continue
		}
		price := entry[0]
		amount := entry[2]

		isBid := amount > 0
		absSize := math.Abs(amount)

		item := models.BookItem{
			Symbol:         normalized,
			ProviderID:     c.settings.ProviderID,
			IsBid:          isBid,
			Price:          floatPtr(price),
			Size:           floatPtr(absSize),
			LocalTimestamp:  now,
			ServerTimestamp: now,
		}

		if isBid {
			bids = append(bids, item)
		} else {
			asks = append(asks, item)
		}
	}

	ob.LoadData(asks, bids)
	c.PublishOrderBook(ob)
}

// processBookUpdate applies a single order book update.
func (c *Connector) processBookUpdate(ob *models.OrderBook, update []float64, normalized string) {
	price := update[0]
	count := update[1]
	amount := update[2]

	now := time.Now()
	isBid := amount > 0
	absSize := math.Abs(amount)

	isBidPtr := boolPtr(isBid)

	if count == 0 {
		// Delete the level.
		ob.DeleteLevel(models.DeltaBookItem{
			IsBid:          isBidPtr,
			Price:          floatPtr(price),
			Size:           floatPtr(0),
			LocalTimestamp:  now,
			ServerTimestamp: now,
		})
	} else {
		// Add or update.
		ob.AddOrUpdateLevel(models.DeltaBookItem{
			IsBid:          isBidPtr,
			Price:          floatPtr(price),
			Size:           floatPtr(absSize),
			LocalTimestamp:  now,
			ServerTimestamp: now,
		})
	}

	c.PublishOrderBook(ob)
}

// handleTradeData processes trade messages from Bitfinex.
func (c *Connector) handleTradeData(bfxSymbol string, parts []json.RawMessage) {
	if len(parts) == 0 {
		return
	}

	exchangeSym := strings.TrimPrefix(bfxSymbol, "t")
	normalized := c.GetNormalizedSymbol(exchangeSym)

	// Trade execution update: the first part after channel ID could be "te"/"tu" string.
	// Format: [CHANNEL_ID, "te", [ID, MTS, AMOUNT, PRICE]]
	// Or snapshot: [CHANNEL_ID, [[ID, MTS, AMOUNT, PRICE], ...]]

	// Check if first element is a string like "te" or "tu".
	var msgType string
	if json.Unmarshal(parts[0], &msgType) == nil {
		if msgType == "te" && len(parts) > 1 {
			var tradeArr []float64
			if err := json.Unmarshal(parts[1], &tradeArr); err == nil && len(tradeArr) >= 4 {
				c.publishTrade(normalized, tradeArr)
			}
		}
		// "tu" (trade update) is ignored to avoid duplicates.
		return
	}

	// Snapshot: array of trade arrays.
	var tradeSnapshot [][]float64
	if err := json.Unmarshal(parts[0], &tradeSnapshot); err == nil {
		// Trade snapshots are historical; we skip them.
		return
	}
}

// publishTrade converts a Bitfinex trade array to a models.Trade and publishes it.
func (c *Connector) publishTrade(normalized string, tradeArr []float64) {
	// [ID, MTS, AMOUNT, PRICE]
	mts := tradeArr[1]
	amount := tradeArr[2]
	price := tradeArr[3]

	isBuy := amount > 0
	absSize := math.Abs(amount)

	ts := time.UnixMilli(int64(mts))

	trade := models.Trade{
		ProviderID:   c.settings.ProviderID,
		ProviderName: c.settings.ProviderName,
		Symbol:       normalized,
		Price:        decimal.NewFromFloat(price),
		Size:         decimal.NewFromFloat(absSize),
		Timestamp:    ts,
		IsBuy:        boolPtr(isBuy),
	}

	c.PublishTrade(trade)
}

func floatPtr(v float64) *float64 { return &v }
func boolPtr(v bool) *bool        { return &v }
