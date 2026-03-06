package gemini

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

// GeminiConnector is a market data connector for the Gemini exchange.
// It maintains two separate WebSocket connections: one for L2 market data
// and one for private user order events.
type GeminiConnector struct {
	*connector.BaseConnector

	settings Settings
	logger   *slog.Logger
	bus      *eventbus.Bus

	marketConn  *websocket.Conn
	privateConn *websocket.Conn

	orderBooks map[string]*models.OrderBook // keyed by exchange symbol
	obMu       sync.RWMutex

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates a new GeminiConnector with the given settings, event bus, and
// logger. The BaseConnector is initialised with metadata from the settings.
func New(settings Settings, bus *eventbus.Bus, logger *slog.Logger) *GeminiConnector {
	if logger == nil {
		logger = slog.Default()
	}

	bc := connector.NewBaseConnector(connector.BaseConnectorConfig{
		Name:         "Gemini",
		Version:      "1.0.0",
		Description:  "Gemini exchange connector",
		Author:       "VisualHFT",
		ProviderID:   settings.ProviderID,
		ProviderName: settings.ProviderName,
		Bus:          bus,
		Logger:       logger,
	})

	gc := &GeminiConnector{
		BaseConnector: bc,
		settings:      settings,
		logger:        logger,
		bus:           bus,
		orderBooks:    make(map[string]*models.OrderBook),
	}

	// Register reconnection action.
	bc.SetReconnectionAction(gc.reconnect)

	return gc
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// StartAsync initiates both WebSocket connections and begins reading messages.
func (gc *GeminiConnector) StartAsync(ctx context.Context) error {
	if err := gc.BaseConnector.StartAsync(ctx); err != nil {
		return err
	}

	// Parse symbols from the settings.
	gc.parseSymbols()

	// Initialise order books for each symbol.
	gc.initOrderBooks()

	ctx, gc.cancel = context.WithCancel(ctx)

	// Connect market data WebSocket.
	if err := gc.connectMarket(ctx); err != nil {
		return fmt.Errorf("gemini: market ws connect: %w", err)
	}

	gc.SetStatus(enums.PluginStarted)
	gc.PublishProvider(gc.GetProviderModel(enums.SessionConnected))

	return nil
}

// StopAsync gracefully shuts down both WebSocket connections and waits for
// read loops to exit.
func (gc *GeminiConnector) StopAsync(ctx context.Context) error {
	if gc.cancel != nil {
		gc.cancel()
	}

	if gc.marketConn != nil {
		gc.marketConn.Close()
	}
	if gc.privateConn != nil {
		gc.privateConn.Close()
	}

	gc.wg.Wait()

	return gc.BaseConnector.StopAsync(ctx)
}

// ---------------------------------------------------------------------------
// Symbol parsing & order book initialisation
// ---------------------------------------------------------------------------

func (gc *GeminiConnector) parseSymbols() {
	gc.BaseConnector.ParseSymbols(strings.Join(gc.settings.Symbols, ","))
}

func (gc *GeminiConnector) initOrderBooks() {
	gc.obMu.Lock()
	defer gc.obMu.Unlock()

	for _, sym := range gc.BaseConnector.GetAllExchangeSymbols() {
		depth := gc.settings.DepthLevels
		if depth <= 0 {
			depth = 20
		}
		ob := models.NewOrderBook(gc.BaseConnector.GetNormalizedSymbol(sym), 2, depth)
		ob.ProviderID = gc.settings.ProviderID
		ob.ProviderName = gc.settings.ProviderName
		gc.orderBooks[sym] = ob
	}
}

func (gc *GeminiConnector) getOrderBook(exchangeSymbol string) *models.OrderBook {
	gc.obMu.RLock()
	defer gc.obMu.RUnlock()
	return gc.orderBooks[exchangeSymbol]
}

// ---------------------------------------------------------------------------
// Market data WebSocket
// ---------------------------------------------------------------------------

func (gc *GeminiConnector) connectMarket(ctx context.Context) error {
	wsURL := gc.settings.WebSocketHostName
	if !strings.Contains(wsURL, "heartbeat") {
		if strings.Contains(wsURL, "?") {
			wsURL += "&heartbeat=true"
		} else {
			wsURL += "?heartbeat=true"
		}
	}

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("gemini: dial market ws: %w", err)
	}
	gc.marketConn = conn

	// Subscribe to L2 data for all exchange symbols.
	if err := gc.subscribeMarket(); err != nil {
		conn.Close()
		return fmt.Errorf("gemini: subscribe market: %w", err)
	}

	// Start the read loop.
	gc.wg.Add(1)
	go gc.readMarketLoop(ctx)

	return nil
}

// subscribeMessage is the JSON structure for Gemini v2 subscription.
type subscribeMessage struct {
	Type          string         `json:"type"`
	Subscriptions []subscription `json:"subscriptions"`
}

type subscription struct {
	Name    string   `json:"name"`
	Symbols []string `json:"symbols"`
}

func (gc *GeminiConnector) subscribeMarket() error {
	symbols := gc.BaseConnector.GetAllExchangeSymbols()
	msg := subscribeMessage{
		Type: "subscribe",
		Subscriptions: []subscription{
			{
				Name:    "l2",
				Symbols: symbols,
			},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("gemini: marshal subscribe: %w", err)
	}

	return gc.marketConn.WriteMessage(websocket.TextMessage, data)
}

// ---------------------------------------------------------------------------
// Market data read loop & message handling
// ---------------------------------------------------------------------------

// wsMessage is a minimal envelope for determining the message type.
type wsMessage struct {
	Type string `json:"type"`
}

// l2UpdateMessage represents an l2_updates message from Gemini.
type l2UpdateMessage struct {
	Type    string     `json:"type"`
	Symbol  string     `json:"symbol"`
	Changes [][]string `json:"changes"` // [["buy"|"sell", price, qty], ...]
	Trades  []wsTrade  `json:"trades"`
}

// wsTrade represents a trade entry embedded in an l2_updates message.
type wsTrade struct {
	Type      string `json:"type"`
	Symbol    string `json:"symbol"`
	Price     string `json:"price"`
	Quantity  string `json:"quantity"`
	Side      string `json:"side"`
	Timestamp int64  `json:"timestamp"`
}

// tradeMessage represents a standalone trade message from Gemini.
type tradeMessage struct {
	Type      string `json:"type"`
	Symbol    string `json:"symbol"`
	Price     string `json:"price"`
	Quantity  string `json:"quantity"`
	Side      string `json:"side"`
	Timestamp int64  `json:"timestamp"`
}

func (gc *GeminiConnector) readMarketLoop(ctx context.Context) {
	defer gc.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_, data, err := gc.marketConn.ReadMessage()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
			}
			gc.logger.Warn("gemini: market ws read error", slog.Any("error", err))
			gc.HandleConnectionLost(ctx, "market ws read error", err)
			return
		}

		gc.handleMarketMessage(data)
	}
}

func (gc *GeminiConnector) handleMarketMessage(data []byte) {
	var env wsMessage
	if err := json.Unmarshal(data, &env); err != nil {
		gc.logger.Warn("gemini: unmarshal envelope", slog.Any("error", err))
		return
	}

	switch env.Type {
	case "heartbeat":
		gc.handleHeartbeat()
	case "l2_updates":
		gc.handleL2Updates(data)
	case "trade":
		gc.handleTrade(data)
	}
}

func (gc *GeminiConnector) handleHeartbeat() {
	gc.PublishProvider(gc.GetProviderModel(enums.SessionConnected))
}

func (gc *GeminiConnector) handleL2Updates(data []byte) {
	var msg l2UpdateMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		gc.logger.Warn("gemini: unmarshal l2_updates", slog.Any("error", err))
		return
	}

	ob := gc.getOrderBook(msg.Symbol)
	if ob == nil {
		gc.logger.Warn("gemini: no order book for symbol", slog.String("symbol", msg.Symbol))
		return
	}

	now := time.Now()

	for _, change := range msg.Changes {
		if len(change) < 3 {
			continue
		}
		side := change[0]
		priceStr := change[1]
		qtyStr := change[2]

		price, err := strconv.ParseFloat(priceStr, 64)
		if err != nil {
			gc.logger.Warn("gemini: parse price", slog.Any("error", err), slog.String("value", priceStr))
			continue
		}

		qty, err := strconv.ParseFloat(qtyStr, 64)
		if err != nil {
			gc.logger.Warn("gemini: parse qty", slog.Any("error", err), slog.String("value", qtyStr))
			continue
		}

		isBid := side == "buy"

		if qty == 0 {
			// Delete level.
			delta := models.DeltaBookItem{
				IsBid:          boolPtr(isBid),
				Price:          &price,
				Size:           &qty,
				LocalTimestamp:  now,
				ServerTimestamp: now,
			}
			ob.DeleteLevel(delta)
		} else {
			// Add or update level.
			delta := models.DeltaBookItem{
				IsBid:          boolPtr(isBid),
				Price:          &price,
				Size:           &qty,
				LocalTimestamp:  now,
				ServerTimestamp: now,
			}
			ob.AddOrUpdateLevel(delta)
		}
	}

	// Process embedded trades.
	for _, t := range msg.Trades {
		gc.publishWsTrade(t)
	}

	// Publish the updated order book.
	gc.PublishOrderBook(ob)
}

func (gc *GeminiConnector) handleTrade(data []byte) {
	var msg tradeMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		gc.logger.Warn("gemini: unmarshal trade", slog.Any("error", err))
		return
	}

	gc.publishTradeFromFields(msg.Symbol, msg.Price, msg.Quantity, msg.Side, msg.Timestamp)
}

func (gc *GeminiConnector) publishWsTrade(t wsTrade) {
	gc.publishTradeFromFields(t.Symbol, t.Price, t.Quantity, t.Side, t.Timestamp)
}

func (gc *GeminiConnector) publishTradeFromFields(symbol, priceStr, qtyStr, side string, ts int64) {
	price, err := decimal.NewFromString(priceStr)
	if err != nil {
		gc.logger.Warn("gemini: parse trade price", slog.Any("error", err))
		return
	}

	qty, err := decimal.NewFromString(qtyStr)
	if err != nil {
		gc.logger.Warn("gemini: parse trade qty", slog.Any("error", err))
		return
	}

	isBuy := side == "buy"
	normalizedSymbol := gc.BaseConnector.GetNormalizedSymbol(symbol)

	var timestamp time.Time
	if ts > 0 {
		timestamp = time.Unix(ts, 0)
	} else {
		timestamp = time.Now()
	}

	trade := models.Trade{
		ProviderID:   gc.settings.ProviderID,
		ProviderName: gc.settings.ProviderName,
		Symbol:       normalizedSymbol,
		Price:        price,
		Size:         qty,
		Timestamp:    timestamp,
		IsBuy:        &isBuy,
	}

	gc.PublishTrade(trade)
}

// ---------------------------------------------------------------------------
// Reconnection
// ---------------------------------------------------------------------------

func (gc *GeminiConnector) reconnect(ctx context.Context) error {
	// Close existing connections.
	if gc.marketConn != nil {
		gc.marketConn.Close()
	}
	if gc.privateConn != nil {
		gc.privateConn.Close()
	}

	// Re-connect market data.
	return gc.connectMarket(ctx)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func boolPtr(b bool) *bool {
	return &b
}
