package coinbase

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
)

const (
	defaultWSURL   = "wss://advanced-trade-ws.coinbase.com"
	defaultRESTURL = "https://api.exchange.coinbase.com"
)

// CoinbaseConnector implements a VisualHFT market data connector for
// the Coinbase Advanced Trade API. It streams L2 order book updates and
// market trades via WebSocket, using a REST snapshot to seed each book.
type CoinbaseConnector struct {
	*connector.BaseConnector

	settings Settings
	logger   *slog.Logger

	wsURL   string // WebSocket URL (overridable for tests)
	restURL string // REST base URL (overridable for tests)

	conn   *websocket.Conn
	connMu sync.Mutex

	orderBooks map[string]*models.OrderBook
	obMu       sync.RWMutex

	cancel context.CancelFunc
	wg     sync.WaitGroup

	// httpClient is the HTTP client used for REST snapshot requests.
	// It can be overridden in tests.
	httpClient *http.Client
}

// New creates a new CoinbaseConnector with the given base connector and
// settings.
func New(base *connector.BaseConnector, settings Settings, logger *slog.Logger) *CoinbaseConnector {
	if logger == nil {
		logger = slog.Default()
	}
	if settings.DepthLevels <= 0 {
		settings.DepthLevels = 25
	}

	c := &CoinbaseConnector{
		BaseConnector: base,
		settings:      settings,
		logger:        logger,
		wsURL:         defaultWSURL,
		restURL:       defaultRESTURL,
		orderBooks:    make(map[string]*models.OrderBook),
		httpClient:    http.DefaultClient,
	}

	// Parse symbols from settings into the base connector's symbol map.
	c.BaseConnector.ParseSymbols(strings.Join(settings.Symbols, ","))

	// Register the reconnection action.
	c.BaseConnector.SetReconnectionAction(c.reconnect)

	return c
}

// StartAsync connects to the Coinbase WebSocket and begins processing
// market data.
func (c *CoinbaseConnector) StartAsync(ctx context.Context) error {
	if err := c.BaseConnector.StartAsync(ctx); err != nil {
		return err
	}

	childCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	if err := c.connect(childCtx); err != nil {
		cancel()
		return fmt.Errorf("coinbase: connect: %w", err)
	}

	c.BaseConnector.SetStatus(enums.PluginStarted)
	c.PublishProvider(c.GetProviderModel(enums.SessionConnected))

	return nil
}

// StopAsync disconnects from the WebSocket and stops all goroutines.
func (c *CoinbaseConnector) StopAsync(ctx context.Context) error {
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

// connect establishes the WebSocket connection, fetches REST snapshots,
// subscribes to channels, and starts the read loop.
func (c *CoinbaseConnector) connect(ctx context.Context) error {
	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, c.wsURL, nil)
	if err != nil {
		return fmt.Errorf("coinbase: dial: %w", err)
	}

	c.connMu.Lock()
	c.conn = conn
	c.connMu.Unlock()

	exchangeSymbols := c.BaseConnector.GetAllExchangeSymbols()

	// Fetch REST snapshots for each symbol.
	for _, sym := range exchangeSymbols {
		if err := c.fetchSnapshot(ctx, sym); err != nil {
			c.logger.Warn("failed to fetch snapshot",
				slog.String("symbol", sym),
				slog.Any("error", err),
			)
		}
	}

	// Subscribe to L2 and market_trades channels.
	if err := c.subscribe(conn, exchangeSymbols); err != nil {
		conn.Close()
		return fmt.Errorf("coinbase: subscribe: %w", err)
	}

	// Start the read loop.
	c.wg.Add(1)
	go c.readLoop(ctx)

	return nil
}

// reconnect is the function passed to BaseConnector.SetReconnectionAction.
func (c *CoinbaseConnector) reconnect(ctx context.Context) error {
	return c.connect(ctx)
}

// subscribe sends subscription messages for l2 and market_trades channels.
func (c *CoinbaseConnector) subscribe(conn *websocket.Conn, symbols []string) error {
	// Subscribe to level2 channel.
	l2Sub := subscribeMsg{
		Type:       "subscribe",
		ProductIDs: symbols,
		Channel:    "level2",
	}
	if err := conn.WriteJSON(l2Sub); err != nil {
		return fmt.Errorf("coinbase: subscribe level2: %w", err)
	}

	// Subscribe to market_trades channel.
	tradeSub := subscribeMsg{
		Type:       "subscribe",
		ProductIDs: symbols,
		Channel:    "market_trades",
	}
	if err := conn.WriteJSON(tradeSub); err != nil {
		return fmt.Errorf("coinbase: subscribe market_trades: %w", err)
	}

	return nil
}

// fetchSnapshot retrieves a REST order book snapshot for a symbol.
func (c *CoinbaseConnector) fetchSnapshot(ctx context.Context, symbol string) error {
	url := fmt.Sprintf("%s/products/%s/book?level=2", c.restURL, symbol)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("coinbase: create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("coinbase: http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("coinbase: snapshot HTTP %d: %s", resp.StatusCode, string(body))
	}

	var snapshot restSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		return fmt.Errorf("coinbase: decode snapshot: %w", err)
	}

	normalizedSymbol := c.BaseConnector.GetNormalizedSymbol(symbol)

	// Parse bids.
	bids := make([]models.BookItem, 0, len(snapshot.Bids))
	for _, entry := range snapshot.Bids {
		if len(entry) < 2 {
			continue
		}
		price, err := strconv.ParseFloat(entry[0].(string), 64)
		if err != nil {
			continue
		}
		size, err := strconv.ParseFloat(entry[1].(string), 64)
		if err != nil {
			continue
		}
		p := price
		s := size
		bids = append(bids, models.BookItem{
			Price: &p,
			Size:  &s,
			IsBid: true,
		})
	}

	// Parse asks.
	asks := make([]models.BookItem, 0, len(snapshot.Asks))
	for _, entry := range snapshot.Asks {
		if len(entry) < 2 {
			continue
		}
		price, err := strconv.ParseFloat(entry[0].(string), 64)
		if err != nil {
			continue
		}
		size, err := strconv.ParseFloat(entry[1].(string), 64)
		if err != nil {
			continue
		}
		p := price
		s := size
		asks = append(asks, models.BookItem{
			Price: &p,
			Size:  &s,
			IsBid: false,
		})
	}

	// Detect decimal places from prices.
	var prices []float64
	for i := range bids {
		if bids[i].Price != nil {
			prices = append(prices, *bids[i].Price)
		}
	}
	for i := range asks {
		if asks[i].Price != nil {
			prices = append(prices, *asks[i].Price)
		}
	}
	decPlaces := connector.RecognizeDecimalPlaces(prices)

	ob := models.NewOrderBook(normalizedSymbol, decPlaces, c.settings.DepthLevels)
	ob.ProviderID = c.settings.ProviderID
	ob.ProviderName = c.settings.ProviderName
	ob.ProviderStatus = enums.SessionConnected
	ob.Sequence = snapshot.Sequence

	ob.LoadData(asks, bids)

	c.obMu.Lock()
	c.orderBooks[symbol] = ob
	c.obMu.Unlock()

	c.PublishOrderBook(ob)

	c.logger.Info("snapshot loaded",
		slog.String("symbol", symbol),
		slog.Int("bids", len(bids)),
		slog.Int("asks", len(asks)),
		slog.Int64("sequence", snapshot.Sequence),
	)

	return nil
}

// readLoop reads messages from the WebSocket and dispatches them.
func (c *CoinbaseConnector) readLoop(ctx context.Context) {
	defer c.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		c.connMu.Lock()
		conn := c.conn
		c.connMu.Unlock()

		if conn == nil {
			return
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			c.logger.Warn("websocket read error", slog.Any("error", err))
			c.BaseConnector.HandleConnectionLost(ctx, "websocket read error", err)
			return
		}

		c.handleMessage(message)
	}
}

// handleMessage routes a raw WebSocket message to the appropriate handler.
func (c *CoinbaseConnector) handleMessage(data []byte) {
	var msg wsMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		c.logger.Warn("failed to unmarshal message", slog.Any("error", err))
		return
	}

	switch msg.Channel {
	case "l2_data":
		c.handleL2Data(msg)
	case "market_trades":
		c.handleMarketTrades(msg)
	default:
		// Ignore other channels (subscriptions, heartbeats, etc.)
	}
}

// handleL2Data processes L2 order book update events.
func (c *CoinbaseConnector) handleL2Data(msg wsMessage) {
	for _, event := range msg.Events {
		// Ignore snapshot events; we use REST snapshots instead.
		if event.Type == "snapshot" {
			continue
		}

		productID := event.ProductID
		if productID == "" && len(event.Updates) > 0 {
			// Some messages nest product_id inside updates.
			productID = event.Updates[0].ProductID
		}
		if productID == "" {
			continue
		}

		c.obMu.RLock()
		ob, exists := c.orderBooks[productID]
		c.obMu.RUnlock()

		if !exists {
			continue
		}

		now := time.Now()
		for _, update := range event.Updates {
			price, err := strconv.ParseFloat(update.PriceLevel, 64)
			if err != nil {
				continue
			}
			qty, err := strconv.ParseFloat(update.NewQuantity, 64)
			if err != nil {
				continue
			}

			isBid := update.Side == "bid"
			bidPtr := &isBid
			pricePtr := &price

			if qty == 0 {
				// Delete the level.
				ob.DeleteLevel(models.DeltaBookItem{
					IsBid:          bidPtr,
					Price:          pricePtr,
					LocalTimestamp: now,
				})
			} else {
				// Add or update the level.
				sizePtr := &qty
				ob.AddOrUpdateLevel(models.DeltaBookItem{
					IsBid:          bidPtr,
					Price:          pricePtr,
					Size:           sizePtr,
					LocalTimestamp: now,
				})
			}
		}

		c.PublishOrderBook(ob)
	}
}

// handleMarketTrades processes market trade events.
func (c *CoinbaseConnector) handleMarketTrades(msg wsMessage) {
	for _, event := range msg.Events {
		for _, t := range event.Trades {
			normalizedSymbol := c.BaseConnector.GetNormalizedSymbol(t.ProductID)
			price, err := decimal.NewFromString(t.Price)
			if err != nil {
				c.logger.Warn("invalid trade price",
					slog.String("price", t.Price),
					slog.Any("error", err),
				)
				continue
			}
			size, err := decimal.NewFromString(t.Size)
			if err != nil {
				c.logger.Warn("invalid trade size",
					slog.String("size", t.Size),
					slog.Any("error", err),
				)
				continue
			}

			isBuy := strings.EqualFold(t.Side, "BUY")

			ts, err := time.Parse(time.RFC3339Nano, t.Time)
			if err != nil {
				ts = time.Now()
			}

			trade := models.Trade{
				ProviderID:   c.settings.ProviderID,
				ProviderName: c.settings.ProviderName,
				Symbol:       normalizedSymbol,
				Price:        price,
				Size:         size,
				Timestamp:    ts,
				IsBuy:        &isBuy,
			}

			c.PublishTrade(trade)
		}
	}
}

// ---------------------------------------------------------------------------
// WebSocket message types
// ---------------------------------------------------------------------------

type subscribeMsg struct {
	Type       string   `json:"type"`
	ProductIDs []string `json:"product_ids"`
	Channel    string   `json:"channel"`
}

type wsMessage struct {
	Channel string    `json:"channel"`
	Events  []wsEvent `json:"events"`
}

type wsEvent struct {
	Type      string     `json:"type"`
	ProductID string     `json:"product_id"`
	Updates   []l2Update `json:"updates"`
	Trades    []wsTrade  `json:"trades"`
}

type l2Update struct {
	Side        string `json:"side"`
	PriceLevel  string `json:"price_level"`
	NewQuantity string `json:"new_quantity"`
	ProductID   string `json:"product_id"`
}

type wsTrade struct {
	ProductID string `json:"product_id"`
	Price     string `json:"price"`
	Size      string `json:"size"`
	Side      string `json:"side"`
	Time      string `json:"time"`
}

// ---------------------------------------------------------------------------
// REST snapshot types
// ---------------------------------------------------------------------------

type restSnapshot struct {
	Bids     [][]interface{} `json:"bids"`
	Asks     [][]interface{} `json:"asks"`
	Sequence int64           `json:"sequence"`
}
