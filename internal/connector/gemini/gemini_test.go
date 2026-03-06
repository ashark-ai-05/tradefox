package gemini

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ashark-ai-05/tradefox/internal/core/enums"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

// upgrader is used by the mock WS server to upgrade HTTP connections.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// mockServer starts an httptest.Server that upgrades to WebSocket and calls
// handler for each connection. The returned server URL has the "ws://" scheme.
func mockServer(t *testing.T, handler func(conn *websocket.Conn)) (*httptest.Server, string) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("mock server upgrade error: %v", err)
			return
		}
		defer conn.Close()
		handler(conn)
	}))

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	return srv, wsURL
}

// newTestSetup creates a Settings, Bus, logger, and GeminiConnector suitable
// for testing. The settings use the given wsURL as the market data endpoint.
func newTestSetup(t *testing.T, wsURL string) (Settings, *eventbus.Bus, *GeminiConnector) {
	t.Helper()

	logger := slog.Default()
	bus := eventbus.NewBus(logger)

	settings := DefaultSettings()
	settings.WebSocketHostName = wsURL
	settings.Symbols = []string{"BTCUSD(BTC/USD)"}

	gc := New(settings, bus, logger)
	return settings, bus, gc
}

// ---------------------------------------------------------------------------
// 1. TestGemini_Settings_Defaults
// ---------------------------------------------------------------------------

func TestGemini_Settings_Defaults(t *testing.T) {
	s := DefaultSettings()

	if s.HostName != "https://api.gemini.com/v1/book/" {
		t.Errorf("unexpected HostName: %s", s.HostName)
	}
	if s.WebSocketHostName != "wss://api.gemini.com/v2/marketdata" {
		t.Errorf("unexpected WebSocketHostName: %s", s.WebSocketHostName)
	}
	if s.WebSocketHostNameUserOrder != "wss://api.gemini.com/v1/order/events" {
		t.Errorf("unexpected WebSocketHostNameUserOrder: %s", s.WebSocketHostNameUserOrder)
	}
	if s.DepthLevels != 20 {
		t.Errorf("expected DepthLevels 20, got %d", s.DepthLevels)
	}
	if s.ProviderID != 5 {
		t.Errorf("expected ProviderID 5, got %d", s.ProviderID)
	}
	if s.ProviderName != "Gemini" {
		t.Errorf("expected ProviderName Gemini, got %s", s.ProviderName)
	}
}

// ---------------------------------------------------------------------------
// 2. TestGemini_Subscribe
// ---------------------------------------------------------------------------

func TestGemini_Subscribe(t *testing.T) {
	var receivedMsg []byte
	var wg sync.WaitGroup
	wg.Add(1)

	srv, wsURL := mockServer(t, func(conn *websocket.Conn) {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Logf("mock server read error: %v", err)
			wg.Done()
			return
		}
		receivedMsg = msg
		wg.Done()

		// Keep connection alive until test finishes.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer srv.Close()

	_, _, gc := newTestSetup(t, wsURL)

	ctx, cancel := t.Context(), func() {} // use test context
	_ = cancel

	err := gc.StartAsync(ctx)
	if err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer gc.StopAsync(ctx)

	wg.Wait()

	// Verify the subscription message.
	var sub subscribeMessage
	if err := json.Unmarshal(receivedMsg, &sub); err != nil {
		t.Fatalf("unmarshal subscription: %v", err)
	}

	if sub.Type != "subscribe" {
		t.Errorf("expected type 'subscribe', got %q", sub.Type)
	}
	if len(sub.Subscriptions) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(sub.Subscriptions))
	}
	if sub.Subscriptions[0].Name != "l2" {
		t.Errorf("expected name 'l2', got %q", sub.Subscriptions[0].Name)
	}
	if len(sub.Subscriptions[0].Symbols) != 1 || sub.Subscriptions[0].Symbols[0] != "BTCUSD" {
		t.Errorf("unexpected symbols: %v", sub.Subscriptions[0].Symbols)
	}
}

// ---------------------------------------------------------------------------
// 3. TestGemini_L2Updates
// ---------------------------------------------------------------------------

func TestGemini_L2Updates(t *testing.T) {
	srv, wsURL := mockServer(t, func(conn *websocket.Conn) {
		// Read subscription message.
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}

		// Send an l2_updates message with bids and asks.
		msg := `{
			"type": "l2_updates",
			"symbol": "BTCUSD",
			"changes": [
				["buy", "30000.00", "0.5"],
				["buy", "29999.00", "1.0"],
				["sell", "30001.00", "0.75"],
				["sell", "30002.00", "2.0"]
			],
			"trades": []
		}`
		if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
			return
		}

		// Keep alive.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer srv.Close()

	_, bus, gc := newTestSetup(t, wsURL)

	_, obCh := bus.OrderBooks.Subscribe(16)

	ctx := t.Context()
	if err := gc.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer gc.StopAsync(ctx)

	// Wait for an order book publish.
	select {
	case ob := <-obCh:
		if ob.Symbol != "BTC/USD" {
			t.Errorf("expected symbol BTC/USD, got %s", ob.Symbol)
		}

		bids := ob.Bids()
		if len(bids) < 2 {
			t.Fatalf("expected at least 2 bids, got %d", len(bids))
		}
		// Best bid should be 30000 (highest).
		if bids[0].Price == nil || *bids[0].Price != 30000.00 {
			t.Errorf("expected best bid 30000.00, got %v", bids[0].Price)
		}

		asks := ob.Asks()
		if len(asks) < 2 {
			t.Fatalf("expected at least 2 asks, got %d", len(asks))
		}
		// Best ask should be 30001 (lowest).
		if asks[0].Price == nil || *asks[0].Price != 30001.00 {
			t.Errorf("expected best ask 30001.00, got %v", asks[0].Price)
		}

	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for order book update")
	}
}

// ---------------------------------------------------------------------------
// 4. TestGemini_Trade
// ---------------------------------------------------------------------------

func TestGemini_Trade(t *testing.T) {
	srv, wsURL := mockServer(t, func(conn *websocket.Conn) {
		// Read subscription message.
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}

		// Send a standalone trade message.
		msg := `{
			"type": "trade",
			"symbol": "BTCUSD",
			"price": "30000.50",
			"quantity": "0.25",
			"side": "buy",
			"timestamp": 1234567890
		}`
		if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
			return
		}

		// Keep alive.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer srv.Close()

	_, bus, gc := newTestSetup(t, wsURL)

	_, tradeCh := bus.Trades.Subscribe(16)

	ctx := t.Context()
	if err := gc.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer gc.StopAsync(ctx)

	// Wait for a trade publish.
	select {
	case trade := <-tradeCh:
		if trade.Symbol != "BTC/USD" {
			t.Errorf("expected symbol BTC/USD, got %s", trade.Symbol)
		}
		if trade.Price.String() != "30000.5" {
			t.Errorf("expected price 30000.5, got %s", trade.Price.String())
		}
		if trade.Size.String() != "0.25" {
			t.Errorf("expected size 0.25, got %s", trade.Size.String())
		}
		if trade.IsBuy == nil || !*trade.IsBuy {
			t.Error("expected IsBuy=true")
		}
		if trade.ProviderID != 5 {
			t.Errorf("expected providerID 5, got %d", trade.ProviderID)
		}
		expectedTs := time.Unix(1234567890, 0)
		if !trade.Timestamp.Equal(expectedTs) {
			t.Errorf("expected timestamp %v, got %v", expectedTs, trade.Timestamp)
		}

	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for trade")
	}
}

// ---------------------------------------------------------------------------
// 5. TestGemini_Heartbeat
// ---------------------------------------------------------------------------

func TestGemini_Heartbeat(t *testing.T) {
	srv, wsURL := mockServer(t, func(conn *websocket.Conn) {
		// Read subscription message.
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}

		// Send a heartbeat.
		msg := `{"type": "heartbeat"}`
		if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
			return
		}

		// Keep alive.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer srv.Close()

	_, bus, gc := newTestSetup(t, wsURL)

	_, provCh := bus.Providers.Subscribe(16)

	ctx := t.Context()
	if err := gc.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer gc.StopAsync(ctx)

	// The StartAsync publishes a Connected status, and the heartbeat should
	// also publish Connected. We look for at least one Connected event from
	// the heartbeat (after the initial one from StartAsync).
	deadline := time.After(5 * time.Second)
	connectedCount := 0
	for connectedCount < 2 {
		select {
		case prov := <-provCh:
			if prov.Status == enums.SessionConnected {
				connectedCount++
			}
		case <-deadline:
			t.Fatalf("timed out waiting for heartbeat provider status, got %d Connected events", connectedCount)
		}
	}

	// At least 2 Connected events: one from StartAsync, one from heartbeat.
	if connectedCount < 2 {
		t.Errorf("expected at least 2 Connected events, got %d", connectedCount)
	}
}

// ---------------------------------------------------------------------------
// 6. TestGemini_DeleteLevel
// ---------------------------------------------------------------------------

func TestGemini_DeleteLevel(t *testing.T) {
	step := make(chan struct{})

	srv, wsURL := mockServer(t, func(conn *websocket.Conn) {
		// Read subscription message.
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}

		// Step 1: Populate the order book with a bid and ask.
		msg1 := `{
			"type": "l2_updates",
			"symbol": "BTCUSD",
			"changes": [
				["buy", "30000.00", "0.5"],
				["sell", "30001.00", "0.75"]
			],
			"trades": []
		}`
		if err := conn.WriteMessage(websocket.TextMessage, []byte(msg1)); err != nil {
			return
		}

		// Wait for test to confirm first update received.
		<-step

		// Step 2: Delete the bid level by sending qty=0.
		msg2 := `{
			"type": "l2_updates",
			"symbol": "BTCUSD",
			"changes": [
				["buy", "30000.00", "0"]
			],
			"trades": []
		}`
		if err := conn.WriteMessage(websocket.TextMessage, []byte(msg2)); err != nil {
			return
		}

		// Keep alive.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer srv.Close()

	_, bus, gc := newTestSetup(t, wsURL)

	_, obCh := bus.OrderBooks.Subscribe(16)

	ctx := t.Context()
	if err := gc.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer gc.StopAsync(ctx)

	// Step 1: Wait for the first order book update (populated).
	select {
	case ob := <-obCh:
		bids := ob.Bids()
		if len(bids) != 1 {
			t.Fatalf("step 1: expected 1 bid, got %d", len(bids))
		}
		if bids[0].Price == nil || *bids[0].Price != 30000.00 {
			t.Errorf("step 1: expected bid at 30000.00, got %v", bids[0].Price)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("step 1: timed out waiting for order book update")
	}

	// Signal the server to send the delete.
	close(step)

	// Step 2: Wait for the second order book update (after delete).
	select {
	case ob := <-obCh:
		bids := ob.Bids()
		if len(bids) != 0 {
			t.Errorf("step 2: expected 0 bids after delete, got %d", len(bids))
			for i, b := range bids {
				t.Logf("  bid[%d]: price=%v size=%v", i, b.Price, b.Size)
			}
		}

		// Ask should still be present.
		asks := ob.Asks()
		if len(asks) != 1 {
			t.Errorf("step 2: expected 1 ask, got %d", len(asks))
		}

	case <-time.After(5 * time.Second):
		t.Fatal("step 2: timed out waiting for order book update after delete")
	}
}

// ---------------------------------------------------------------------------
// TestGemini_L2Updates_EmbeddedTrade verifies that trades embedded in
// l2_updates messages are published to the trades topic.
// ---------------------------------------------------------------------------

func TestGemini_L2Updates_EmbeddedTrade(t *testing.T) {
	srv, wsURL := mockServer(t, func(conn *websocket.Conn) {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}

		msg := `{
			"type": "l2_updates",
			"symbol": "BTCUSD",
			"changes": [
				["buy", "30000.00", "0.5"]
			],
			"trades": [
				{
					"type": "trade",
					"symbol": "BTCUSD",
					"price": "30000.00",
					"quantity": "0.1",
					"side": "sell",
					"timestamp": 1700000000
				}
			]
		}`
		if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
			return
		}

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer srv.Close()

	_, bus, gc := newTestSetup(t, wsURL)

	_, tradeCh := bus.Trades.Subscribe(16)

	ctx := t.Context()
	if err := gc.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer gc.StopAsync(ctx)

	select {
	case trade := <-tradeCh:
		if trade.Symbol != "BTC/USD" {
			t.Errorf("expected symbol BTC/USD, got %s", trade.Symbol)
		}
		if trade.IsBuy == nil || *trade.IsBuy {
			t.Error("expected IsBuy=false for sell trade")
		}

	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for embedded trade")
	}
}

// ---------------------------------------------------------------------------
// TestGemini_ConnectorMetadata verifies the base connector metadata fields.
// ---------------------------------------------------------------------------

func TestGemini_ConnectorMetadata(t *testing.T) {
	logger := slog.Default()
	bus := eventbus.NewBus(logger)
	settings := DefaultSettings()

	gc := New(settings, bus, logger)

	if gc.Name() != "Gemini" {
		t.Errorf("expected name Gemini, got %s", gc.Name())
	}
	if gc.Version() != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", gc.Version())
	}
	if gc.PluginType() != enums.PluginTypeMarketConnector {
		t.Errorf("expected PluginTypeMarketConnector, got %v", gc.PluginType())
	}
	if gc.Status() != enums.PluginLoaded {
		t.Errorf("expected initial status PluginLoaded, got %v", gc.Status())
	}

	// Verify unused fields are present to ensure the interface is satisfied.
	_ = gc.Description()
	_ = gc.Author()
	_ = gc.PluginUniqueID()
	_ = gc.RequiredLicenseLevel()
}
