package coinbase

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ashark-ai-05/tradefox/internal/connector"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// mockWSServer creates a test WebSocket server using the provided handler.
// It returns the server and its ws:// URL.
func mockWSServer(t *testing.T, handler func(conn *websocket.Conn)) (*httptest.Server, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("upgrade error: %v", err)
			return
		}
		defer conn.Close()
		handler(conn)
	}))
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	return srv, wsURL
}

// mockRESTServer creates a test HTTP server that serves order book snapshots.
func mockRESTServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		snapshot := map[string]interface{}{
			"bids":     [][]interface{}{{"30000.00", "1.5", 3}, {"29999.00", "2.0", 5}},
			"asks":     [][]interface{}{{"30001.00", "0.8", 2}, {"30002.00", "1.2", 4}},
			"sequence": 12345,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(snapshot)
	}))
}

// newTestConnector creates a CoinbaseConnector wired to mock servers.
func newTestConnector(t *testing.T, wsURL, restURL string) (*CoinbaseConnector, *eventbus.Bus) {
	t.Helper()
	logger := slog.Default()
	bus := eventbus.NewBus(logger)
	base := connector.NewBaseConnector(connector.BaseConnectorConfig{
		Name:         "Coinbase",
		Version:      "1.0.0",
		Description:  "Coinbase Advanced Trade connector",
		Author:       "VisualHFT",
		ProviderID:   7,
		ProviderName: "Coinbase",
		Bus:          bus,
		Logger:       logger,
	})

	settings := Settings{
		Symbols:      []string{"BTC-USD(BTC/USD)"},
		DepthLevels:  25,
		ProviderID:   7,
		ProviderName: "Coinbase",
	}

	c := New(base, settings, logger)
	c.wsURL = wsURL
	c.restURL = restURL
	return c, bus
}

// ---------------------------------------------------------------------------
// 1. TestCoinbase_Settings_Defaults
// ---------------------------------------------------------------------------

func TestCoinbase_Settings_Defaults(t *testing.T) {
	s := DefaultSettings()

	if s.DepthLevels != 25 {
		t.Errorf("expected DepthLevels=25, got %d", s.DepthLevels)
	}
	if s.ProviderID != 7 {
		t.Errorf("expected ProviderID=7, got %d", s.ProviderID)
	}
	if s.ProviderName != "Coinbase" {
		t.Errorf("expected ProviderName=Coinbase, got %s", s.ProviderName)
	}
	if s.ApiKey != "" {
		t.Errorf("expected empty ApiKey, got %q", s.ApiKey)
	}
	if s.ApiSecret != "" {
		t.Errorf("expected empty ApiSecret, got %q", s.ApiSecret)
	}
	if len(s.Symbols) != 0 {
		t.Errorf("expected empty Symbols, got %v", s.Symbols)
	}
}

// ---------------------------------------------------------------------------
// 2. TestCoinbase_Subscribe
// ---------------------------------------------------------------------------

func TestCoinbase_Subscribe(t *testing.T) {
	// Track subscription messages received by the mock WS server.
	type subMsg struct {
		Type       string   `json:"type"`
		ProductIDs []string `json:"product_ids"`
		Channel    string   `json:"channel"`
	}

	subscriptionsCh := make(chan subMsg, 10)
	done := make(chan struct{})

	srv, wsURL := mockWSServer(t, func(conn *websocket.Conn) {
		defer close(done)
		for i := 0; i < 2; i++ {
			var msg subMsg
			if err := conn.ReadJSON(&msg); err != nil {
				t.Logf("read subscription error: %v", err)
				return
			}
			subscriptionsCh <- msg
		}
		// Keep connection alive until test ends.
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	})
	defer srv.Close()

	restSrv := mockRESTServer(t)
	defer restSrv.Close()

	c, _ := newTestConnector(t, wsURL, restSrv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer c.StopAsync(context.Background())

	// Collect two subscription messages.
	var subs []subMsg
	timeout := time.After(3 * time.Second)
	for i := 0; i < 2; i++ {
		select {
		case msg := <-subscriptionsCh:
			subs = append(subs, msg)
		case <-timeout:
			t.Fatalf("timed out waiting for subscription messages, got %d", len(subs))
		}
	}

	// Verify we got both channel subscriptions.
	channels := map[string]bool{}
	for _, s := range subs {
		if s.Type != "subscribe" {
			t.Errorf("expected type=subscribe, got %s", s.Type)
		}
		channels[s.Channel] = true
		if len(s.ProductIDs) != 1 || s.ProductIDs[0] != "BTC-USD" {
			t.Errorf("expected product_ids=[BTC-USD], got %v", s.ProductIDs)
		}
	}
	if !channels["level2"] {
		t.Error("missing level2 subscription")
	}
	if !channels["market_trades"] {
		t.Error("missing market_trades subscription")
	}
}

// ---------------------------------------------------------------------------
// 3. TestCoinbase_L2Update
// ---------------------------------------------------------------------------

func TestCoinbase_L2Update(t *testing.T) {
	l2Msg := wsMessage{
		Channel: "l2_data",
		Events: []wsEvent{
			{
				Type:      "update",
				ProductID: "BTC-USD",
				Updates: []l2Update{
					{Side: "bid", PriceLevel: "29500.00", NewQuantity: "3.0"},
					{Side: "ask", PriceLevel: "30500.00", NewQuantity: "1.5"},
				},
			},
		},
	}
	l2Data, _ := json.Marshal(l2Msg)

	msgSent := make(chan struct{})

	srv, wsURL := mockWSServer(t, func(conn *websocket.Conn) {
		// Consume subscription messages.
		for i := 0; i < 2; i++ {
			conn.ReadMessage()
		}
		// Send L2 update.
		conn.WriteMessage(websocket.TextMessage, l2Data)
		close(msgSent)
		// Keep alive.
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	})
	defer srv.Close()

	restSrv := mockRESTServer(t)
	defer restSrv.Close()

	c, bus := newTestConnector(t, wsURL, restSrv.URL)
	_, obCh := bus.OrderBooks.Subscribe(16)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer c.StopAsync(context.Background())

	// Wait for the mock to send the L2 message.
	select {
	case <-msgSent:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for mock to send L2 message")
	}

	// Drain order book events until we find one with the new bid level.
	deadline := time.After(3 * time.Second)
	found := false
	for !found {
		select {
		case ob := <-obCh:
			bids := ob.Bids()
			for _, b := range bids {
				if b.Price != nil && *b.Price == 29500.0 && b.Size != nil && *b.Size == 3.0 {
					found = true
					break
				}
			}
		case <-deadline:
			t.Fatal("timed out waiting for L2 update in order book")
		}
	}

	// Verify the ask side also has the new level.
	c.obMu.RLock()
	ob := c.orderBooks["BTC-USD"]
	c.obMu.RUnlock()

	asks := ob.Asks()
	askFound := false
	for _, a := range asks {
		if a.Price != nil && *a.Price == 30500.0 && a.Size != nil && *a.Size == 1.5 {
			askFound = true
			break
		}
	}
	if !askFound {
		t.Error("ask level 30500.0 @ 1.5 not found in order book")
	}
}

// ---------------------------------------------------------------------------
// 4. TestCoinbase_Trade
// ---------------------------------------------------------------------------

func TestCoinbase_Trade(t *testing.T) {
	tradeMsg := wsMessage{
		Channel: "market_trades",
		Events: []wsEvent{
			{
				Type: "update",
				Trades: []wsTrade{
					{
						ProductID: "BTC-USD",
						Price:     "30123.45",
						Size:      "0.25",
						Side:      "BUY",
						Time:      "2024-01-15T12:30:00Z",
					},
				},
			},
		},
	}
	tradeData, _ := json.Marshal(tradeMsg)

	msgSent := make(chan struct{})

	srv, wsURL := mockWSServer(t, func(conn *websocket.Conn) {
		// Consume subscription messages.
		for i := 0; i < 2; i++ {
			conn.ReadMessage()
		}
		// Send trade.
		conn.WriteMessage(websocket.TextMessage, tradeData)
		close(msgSent)
		// Keep alive.
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	})
	defer srv.Close()

	restSrv := mockRESTServer(t)
	defer restSrv.Close()

	c, bus := newTestConnector(t, wsURL, restSrv.URL)
	_, tradeCh := bus.Trades.Subscribe(16)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer c.StopAsync(context.Background())

	// Wait for mock to send.
	select {
	case <-msgSent:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for mock to send trade message")
	}

	// Read trade from bus.
	var trade models.Trade
	select {
	case trade = <-tradeCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for trade event")
	}

	if trade.Symbol != "BTC/USD" {
		t.Errorf("expected symbol BTC/USD, got %s", trade.Symbol)
	}
	if trade.Price.String() != "30123.45" {
		t.Errorf("expected price 30123.45, got %s", trade.Price.String())
	}
	if trade.Size.String() != "0.25" {
		t.Errorf("expected size 0.25, got %s", trade.Size.String())
	}
	if trade.IsBuy == nil || !*trade.IsBuy {
		t.Error("expected IsBuy=true")
	}
	if trade.ProviderID != 7 {
		t.Errorf("expected providerID=7, got %d", trade.ProviderID)
	}
	if trade.ProviderName != "Coinbase" {
		t.Errorf("expected providerName=Coinbase, got %s", trade.ProviderName)
	}

	expectedTime, _ := time.Parse(time.RFC3339, "2024-01-15T12:30:00Z")
	if !trade.Timestamp.Equal(expectedTime) {
		t.Errorf("expected timestamp %v, got %v", expectedTime, trade.Timestamp)
	}
}

// ---------------------------------------------------------------------------
// 5. TestCoinbase_DeleteLevel
// ---------------------------------------------------------------------------

func TestCoinbase_DeleteLevel(t *testing.T) {
	// First, send an L2 update that adds a bid level.
	// Then send another L2 update with qty=0 to delete it.
	addMsg := wsMessage{
		Channel: "l2_data",
		Events: []wsEvent{
			{
				Type:      "update",
				ProductID: "BTC-USD",
				Updates: []l2Update{
					{Side: "bid", PriceLevel: "28000.00", NewQuantity: "5.0"},
				},
			},
		},
	}
	addData, _ := json.Marshal(addMsg)

	deleteMsg := wsMessage{
		Channel: "l2_data",
		Events: []wsEvent{
			{
				Type:      "update",
				ProductID: "BTC-USD",
				Updates: []l2Update{
					{Side: "bid", PriceLevel: "28000.00", NewQuantity: "0"},
				},
			},
		},
	}
	deleteData, _ := json.Marshal(deleteMsg)

	addSent := make(chan struct{})
	deleteSent := make(chan struct{})

	srv, wsURL := mockWSServer(t, func(conn *websocket.Conn) {
		// Consume subscription messages.
		for i := 0; i < 2; i++ {
			conn.ReadMessage()
		}
		// Send add.
		conn.WriteMessage(websocket.TextMessage, addData)
		close(addSent)
		// Small delay so the add is processed first.
		time.Sleep(100 * time.Millisecond)
		// Send delete.
		conn.WriteMessage(websocket.TextMessage, deleteData)
		close(deleteSent)
		// Keep alive.
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	})
	defer srv.Close()

	restSrv := mockRESTServer(t)
	defer restSrv.Close()

	c, bus := newTestConnector(t, wsURL, restSrv.URL)
	_, obCh := bus.OrderBooks.Subscribe(32)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer c.StopAsync(context.Background())

	// Wait for the delete message to be sent.
	select {
	case <-deleteSent:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for mock to send delete message")
	}

	// Give a moment for processing.
	time.Sleep(200 * time.Millisecond)

	// Drain all order book events.
	drainTimeout := time.After(2 * time.Second)
drainLoop:
	for {
		select {
		case <-obCh:
		case <-drainTimeout:
			break drainLoop
		default:
			break drainLoop
		}
	}

	// Now check the order book directly: the 28000 level should be gone.
	c.obMu.RLock()
	ob := c.orderBooks["BTC-USD"]
	c.obMu.RUnlock()

	if ob == nil {
		t.Fatal("order book for BTC-USD is nil")
	}

	bids := ob.Bids()
	for _, b := range bids {
		if b.Price != nil && *b.Price == 28000.0 {
			if b.Size != nil && *b.Size > 0 {
				t.Errorf("expected bid level 28000.0 to be deleted, but found size=%f", *b.Size)
			} else {
				t.Error("expected bid level 28000.0 to be fully removed from book")
			}
		}
	}

	// Verify original snapshot levels still exist (30000 bid from REST).
	found30000 := false
	for _, b := range bids {
		if b.Price != nil && *b.Price == 30000.0 {
			found30000 = true
			break
		}
	}
	if !found30000 {
		// Log all bids for debugging.
		for i, b := range bids {
			p, s := 0.0, 0.0
			if b.Price != nil {
				p = *b.Price
			}
			if b.Size != nil {
				s = *b.Size
			}
			t.Logf("bid[%d]: price=%f size=%f", i, p, s)
		}
		t.Error("expected original snapshot bid level 30000.0 to still exist")
	}

	_ = fmt.Sprintf("") // use fmt
}
