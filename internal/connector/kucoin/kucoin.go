package kucoin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
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

// Connector is the KuCoin exchange connector for VisualHFT.
type Connector struct {
	*connector.BaseConnector

	settings Settings
	logger   *slog.Logger
	bus      *eventbus.Bus

	conn   *websocket.Conn
	cancel context.CancelFunc
	done   chan struct{}

	// orderBooks stores per-symbol order books.
	orderBooks sync.Map // map[string]*models.OrderBook

	// sequences tracks per-symbol sequence numbers for gap detection.
	sequences sync.Map // map[string]int64

	// snapshotFn is the function used to fetch REST snapshots.
	// Replaceable for testing.
	snapshotFn func(symbol string) (*snapshotResponse, error)

	// pingDone tracks the ping goroutine.
	pingDone chan struct{}
}

// snapshotResponse represents the REST API order book snapshot response.
type snapshotResponse struct {
	Sequence string     `json:"sequence"`
	Bids     [][]string `json:"bids"` // [["price", "size"], ...]
	Asks     [][]string `json:"asks"` // [["price", "size"], ...]
}

// New creates a new KuCoin Connector. The connector is in the Loaded state
// and must be started with StartAsync.
func New(settings Settings, bus *eventbus.Bus, logger *slog.Logger) *Connector {
	if logger == nil {
		logger = slog.Default()
	}
	if settings.DepthLevels <= 0 {
		settings.DepthLevels = 25
	}

	base := connector.NewBaseConnector(connector.BaseConnectorConfig{
		Name:         "KuCoin",
		Version:      "1.0.0",
		Description:  "KuCoin WebSocket connector",
		Author:       "VisualHFT",
		ProviderID:   settings.ProviderID,
		ProviderName: settings.ProviderName,
		Bus:          bus,
		Logger:       logger,
	})

	c := &Connector{
		BaseConnector: base,
		settings:      settings,
		logger:        logger,
		bus:           bus,
	}

	// Parse symbols: "BTC-USDT(BTC/USDT)" -> exchange="BTC-USDT", normalized="BTC/USDT"
	base.ParseSymbols(strings.Join(settings.Symbols, ","))
	base.SetReconnectionAction(c.connect)

	return c
}

// SetWSURL sets the WebSocket URL directly. Useful for testing.
func (c *Connector) SetWSURL(url string) {
	c.settings.WSURL = url
}

// SetSnapshotFn replaces the REST snapshot function. Useful for testing.
func (c *Connector) SetSnapshotFn(fn func(symbol string) (*snapshotResponse, error)) {
	c.snapshotFn = fn
}

// StartAsync connects to KuCoin, subscribes to channels, and starts
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
	c.pingDone = make(chan struct{})

	go c.readLoop(readCtx)
	go c.pingLoop(readCtx)

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

	if c.pingDone != nil {
		<-c.pingDone
		c.pingDone = nil
	}

	return c.BaseConnector.StopAsync(ctx)
}

// connect dials the KuCoin WebSocket and sends subscription messages.
func (c *Connector) connect(ctx context.Context) error {
	wsURL := c.settings.WSURL
	if wsURL == "" {
		return fmt.Errorf("kucoin: WSURL is not configured")
	}

	c.logger.Info("kucoin connecting", slog.String("url", wsURL))

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("kucoin dial: %w", err)
	}

	c.conn = conn

	// Read the welcome message.
	_, welcomeMsg, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("kucoin read welcome: %w", err)
	}
	c.logger.Info("kucoin welcome received", slog.String("msg", string(welcomeMsg)))

	// Initialize order books and subscribe for each symbol.
	exchangeSymbols := c.GetAllExchangeSymbols()

	for i, exchangeSym := range exchangeSymbols {
		normalized := c.GetNormalizedSymbol(exchangeSym)
		ob := models.NewOrderBook(normalized, 2, c.settings.DepthLevels)
		ob.ProviderID = c.settings.ProviderID
		ob.ProviderName = c.settings.ProviderName
		ob.ProviderStatus = enums.SessionConnected
		c.orderBooks.Store(normalized, ob)

		// Subscribe to level2 order book.
		bookSub := map[string]interface{}{
			"id":             fmt.Sprintf("%d", i*2+1),
			"type":           "subscribe",
			"topic":          fmt.Sprintf("/market/level2:%s", exchangeSym),
			"privateChannel": false,
			"response":       true,
		}
		if err := conn.WriteJSON(bookSub); err != nil {
			return fmt.Errorf("kucoin subscribe book: %w", err)
		}

		// Subscribe to trade matches.
		tradeSub := map[string]interface{}{
			"id":             fmt.Sprintf("%d", i*2+2),
			"type":           "subscribe",
			"topic":          fmt.Sprintf("/market/match:%s", exchangeSym),
			"privateChannel": false,
			"response":       true,
		}
		if err := conn.WriteJSON(tradeSub); err != nil {
			return fmt.Errorf("kucoin subscribe trade: %w", err)
		}
	}

	// Fetch initial snapshots for each symbol.
	if c.snapshotFn != nil {
		for _, exchangeSym := range exchangeSymbols {
			normalized := c.GetNormalizedSymbol(exchangeSym)
			snap, err := c.snapshotFn(exchangeSym)
			if err != nil {
				c.logger.Warn("kucoin: failed to fetch snapshot",
					slog.String("symbol", exchangeSym),
					slog.Any("error", err),
				)
				continue
			}
			c.applySnapshot(normalized, snap)
		}
	}

	c.logger.Info("kucoin connected and subscribed")
	return nil
}

// applySnapshot loads a REST snapshot into the order book.
func (c *Connector) applySnapshot(normalized string, snap *snapshotResponse) {
	obVal, ok := c.orderBooks.Load(normalized)
	if !ok {
		return
	}
	ob := obVal.(*models.OrderBook)

	now := time.Now()
	bids := make([]models.BookItem, 0, len(snap.Bids))
	asks := make([]models.BookItem, 0, len(snap.Asks))

	for _, b := range snap.Bids {
		if len(b) < 2 {
			continue
		}
		price, _ := strconv.ParseFloat(b[0], 64)
		size, _ := strconv.ParseFloat(b[1], 64)
		bids = append(bids, models.BookItem{
			Symbol:         normalized,
			ProviderID:     c.settings.ProviderID,
			IsBid:          true,
			Price:          floatPtr(price),
			Size:           floatPtr(size),
			LocalTimestamp:  now,
			ServerTimestamp: now,
		})
	}

	for _, a := range snap.Asks {
		if len(a) < 2 {
			continue
		}
		price, _ := strconv.ParseFloat(a[0], 64)
		size, _ := strconv.ParseFloat(a[1], 64)
		asks = append(asks, models.BookItem{
			Symbol:         normalized,
			ProviderID:     c.settings.ProviderID,
			IsBid:          false,
			Price:          floatPtr(price),
			Size:           floatPtr(size),
			LocalTimestamp:  now,
			ServerTimestamp: now,
		})
	}

	// Set the sequence from the snapshot.
	seq, _ := strconv.ParseInt(snap.Sequence, 10, 64)
	c.sequences.Store(normalized, seq)

	ob.LoadData(asks, bids)
	c.PublishOrderBook(ob)
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
			c.logger.Warn("kucoin read error", slog.Any("error", err))
			c.HandleConnectionLost(ctx, "read error", err)
			return
		}

		c.handleMessage(ctx, msg)
	}
}

// pingLoop sends periodic ping messages to keep the connection alive.
func (c *Connector) pingLoop(ctx context.Context) {
	defer close(c.pingDone)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if c.conn != nil {
				ping := map[string]interface{}{
					"id":   "ping",
					"type": "ping",
				}
				if err := c.conn.WriteJSON(ping); err != nil {
					c.logger.Warn("kucoin ping error", slog.Any("error", err))
				}
			}
		}
	}
}

// kucoinMsg is the top-level envelope for KuCoin WS messages.
type kucoinMsg struct {
	Type    string          `json:"type"`
	Topic   string          `json:"topic"`
	Subject string          `json:"subject"`
	Data    json.RawMessage `json:"data"`
}

// kucoinL2Data is the data payload for level2 order book updates.
type kucoinL2Data struct {
	SequenceStart int64    `json:"sequenceStart"`
	SequenceEnd   int64    `json:"sequenceEnd"`
	Changes       struct {
		Asks [][]string `json:"asks"` // [["price", "size", "sequence"], ...]
		Bids [][]string `json:"bids"`
	} `json:"changes"`
}

// kucoinTradeData is the data payload for trade match messages.
type kucoinTradeData struct {
	Symbol string `json:"symbol"`
	Price  string `json:"price"`
	Size   string `json:"size"`
	Side   string `json:"side"`
	Time   string `json:"time"` // nanosecond timestamp as string
}

// handleMessage parses a raw KuCoin message and dispatches it.
func (c *Connector) handleMessage(ctx context.Context, raw []byte) {
	var msg kucoinMsg
	if err := json.Unmarshal(raw, &msg); err != nil {
		c.logger.Warn("kucoin: failed to unmarshal message", slog.Any("error", err))
		return
	}

	switch msg.Type {
	case "message":
		if strings.Contains(msg.Topic, "/market/level2:") {
			c.handleL2Update(ctx, msg)
		} else if strings.Contains(msg.Topic, "/market/match:") {
			c.handleTradeMatch(msg)
		}
	case "welcome", "ack", "pong":
		// Ignored.
	}
}

// handleL2Update processes level2 order book updates.
func (c *Connector) handleL2Update(ctx context.Context, msg kucoinMsg) {
	var l2 kucoinL2Data
	if err := json.Unmarshal(msg.Data, &l2); err != nil {
		c.logger.Warn("kucoin: failed to unmarshal l2 data", slog.Any("error", err))
		return
	}

	// Extract exchange symbol from topic: "/market/level2:BTC-USDT" -> "BTC-USDT"
	parts := strings.SplitN(msg.Topic, ":", 2)
	if len(parts) < 2 {
		return
	}
	exchangeSym := parts[1]
	normalized := c.GetNormalizedSymbol(exchangeSym)

	obVal, ok := c.orderBooks.Load(normalized)
	if !ok {
		return
	}
	ob := obVal.(*models.OrderBook)

	// Check sequence gap.
	seqVal, seqOk := c.sequences.Load(normalized)
	if seqOk {
		localSeq := seqVal.(int64)
		if l2.SequenceStart > localSeq+1 {
			c.logger.Warn("kucoin: sequence gap detected, triggering reconnect",
				slog.String("symbol", exchangeSym),
				slog.Int64("expected", localSeq+1),
				slog.Int64("got", l2.SequenceStart),
			)
			c.HandleConnectionLost(ctx, "sequence gap", nil)
			return
		}
	}

	// Update the local sequence.
	c.sequences.Store(normalized, l2.SequenceEnd)

	now := time.Now()

	for _, b := range l2.Changes.Bids {
		if len(b) < 2 {
			continue
		}
		price, _ := strconv.ParseFloat(b[0], 64)
		size, _ := strconv.ParseFloat(b[1], 64)

		isBidPtr := boolPtr(true)
		if size == 0 {
			ob.DeleteLevel(models.DeltaBookItem{
				IsBid:          isBidPtr,
				Price:          floatPtr(price),
				Size:           floatPtr(0),
				LocalTimestamp:  now,
				ServerTimestamp: now,
			})
		} else {
			ob.AddOrUpdateLevel(models.DeltaBookItem{
				IsBid:          isBidPtr,
				Price:          floatPtr(price),
				Size:           floatPtr(size),
				LocalTimestamp:  now,
				ServerTimestamp: now,
			})
		}
	}

	for _, a := range l2.Changes.Asks {
		if len(a) < 2 {
			continue
		}
		price, _ := strconv.ParseFloat(a[0], 64)
		size, _ := strconv.ParseFloat(a[1], 64)

		isBidPtr := boolPtr(false)
		if size == 0 {
			ob.DeleteLevel(models.DeltaBookItem{
				IsBid:          isBidPtr,
				Price:          floatPtr(price),
				Size:           floatPtr(0),
				LocalTimestamp:  now,
				ServerTimestamp: now,
			})
		} else {
			ob.AddOrUpdateLevel(models.DeltaBookItem{
				IsBid:          isBidPtr,
				Price:          floatPtr(price),
				Size:           floatPtr(size),
				LocalTimestamp:  now,
				ServerTimestamp: now,
			})
		}
	}

	c.PublishOrderBook(ob)
}

// handleTradeMatch processes trade match messages.
func (c *Connector) handleTradeMatch(msg kucoinMsg) {
	var td kucoinTradeData
	if err := json.Unmarshal(msg.Data, &td); err != nil {
		c.logger.Warn("kucoin: failed to unmarshal trade data", slog.Any("error", err))
		return
	}

	normalized := c.GetNormalizedSymbol(td.Symbol)
	isBuy := td.Side == "buy"

	price, _ := strconv.ParseFloat(td.Price, 64)
	size, _ := strconv.ParseFloat(td.Size, 64)

	// Parse nanosecond timestamp.
	var ts time.Time
	nanos, err := strconv.ParseInt(td.Time, 10, 64)
	if err != nil {
		ts = time.Now()
	} else {
		ts = time.Unix(0, nanos)
	}

	trade := models.Trade{
		ProviderID:   c.settings.ProviderID,
		ProviderName: c.settings.ProviderName,
		Symbol:       normalized,
		Price:        decimal.NewFromFloat(price),
		Size:         decimal.NewFromFloat(size),
		Timestamp:    ts,
		IsBuy:        boolPtr(isBuy),
	}

	c.PublishTrade(trade)
}

func floatPtr(v float64) *float64 { return &v }
func boolPtr(v bool) *bool        { return &v }
