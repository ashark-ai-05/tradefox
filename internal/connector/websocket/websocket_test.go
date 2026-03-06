package websocket

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
	"github.com/ashark-ai-05/tradefox/internal/core/enums"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

// upgrader is the gorilla/websocket upgrader used by the mock server.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// startMockServer creates an httptest.Server that upgrades to WebSocket and
// calls handler for each accepted connection. The returned cleanup function
// must be called to close the server.
func startMockServer(t *testing.T, handler func(conn *websocket.Conn)) (*httptest.Server, Settings) {
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

	// Parse the test server address into host and port for Settings.
	// httptest.NewServer gives us "http://127.0.0.1:<port>".
	addr := srv.Listener.Addr().String()
	parts := strings.Split(addr, ":")
	host := parts[0]
	var port int
	for _, c := range parts[1] {
		port = port*10 + int(c-'0')
	}
	for _, c := range parts[1][1:] {
		_ = c // already handled above via full loop
	}
	// Re-parse properly.
	port = 0
	for _, c := range parts[len(parts)-1] {
		port = port*10 + int(c-'0')
	}

	settings := Settings{
		HostName:     host,
		Port:         port,
		ProviderID:   3,
		ProviderName: "TestWS",
	}

	return srv, settings
}

// newTestBusAndLogger creates a fresh Bus and logger for testing.
func newTestBusAndLogger() (*eventbus.Bus, *slog.Logger) {
	logger := slog.Default()
	bus := eventbus.NewBus(logger)
	return bus, logger
}

// ---------------------------------------------------------------------------
// 1. TestWebSocket_Connect
// ---------------------------------------------------------------------------

func TestWebSocket_Connect(t *testing.T) {
	connected := make(chan struct{})

	srv, settings := startMockServer(t, func(conn *websocket.Conn) {
		close(connected)
		// Keep connection alive until the test finishes.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer srv.Close()

	bus, logger := newTestBusAndLogger()
	defer bus.Close()

	wsc := New(settings, bus, logger)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := wsc.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer func() { _ = wsc.StopAsync(context.Background()) }()

	select {
	case <-connected:
		// Success: mock server received the connection.
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for WebSocket connection")
	}

	if wsc.Status() != enums.PluginStarted {
		t.Errorf("expected PluginStarted, got %v", wsc.Status())
	}
}

// ---------------------------------------------------------------------------
// 2. TestWebSocket_MarketMessage
// ---------------------------------------------------------------------------

func TestWebSocket_MarketMessage(t *testing.T) {
	ready := make(chan *websocket.Conn, 1)

	srv, settings := startMockServer(t, func(conn *websocket.Conn) {
		ready <- conn
		// Keep alive.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer srv.Close()

	bus, logger := newTestBusAndLogger()
	defer bus.Close()

	_, obCh := bus.OrderBooks.Subscribe(16)

	wsc := New(settings, bus, logger)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := wsc.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer func() { _ = wsc.StopAsync(context.Background()) }()

	// Wait for the mock server to have the connection.
	var serverConn *websocket.Conn
	select {
	case serverConn = <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for server connection")
	}

	// Build a Market envelope with one OrderBook.
	ob := models.OrderBook{
		Symbol:     "BTC/USDT",
		ProviderID: 3,
		MaxDepth:   10,
	}
	data, _ := json.Marshal([]models.OrderBook{ob})
	env := envelope{Type: "Market", Data: data}
	msg, _ := json.Marshal(env)

	if err := serverConn.WriteMessage(websocket.TextMessage, msg); err != nil {
		t.Fatalf("server write failed: %v", err)
	}

	// Wait for the order book to appear on the bus.
	select {
	case received := <-obCh:
		if received.Symbol != "BTC/USDT" {
			t.Errorf("expected symbol BTC/USDT, got %s", received.Symbol)
		}
		if received.ProviderID != 3 {
			t.Errorf("expected providerID 3, got %d", received.ProviderID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for OrderBook event")
	}
}

// ---------------------------------------------------------------------------
// 3. TestWebSocket_TradeMessage
// ---------------------------------------------------------------------------

func TestWebSocket_TradeMessage(t *testing.T) {
	ready := make(chan *websocket.Conn, 1)

	srv, settings := startMockServer(t, func(conn *websocket.Conn) {
		ready <- conn
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer srv.Close()

	bus, logger := newTestBusAndLogger()
	defer bus.Close()

	_, tradeCh := bus.Trades.Subscribe(16)

	wsc := New(settings, bus, logger)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := wsc.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer func() { _ = wsc.StopAsync(context.Background()) }()

	var serverConn *websocket.Conn
	select {
	case serverConn = <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for server connection")
	}

	trade := models.Trade{
		ProviderID:   3,
		ProviderName: "TestWS",
		Symbol:       "ETH/USDT",
		Price:        decimal.NewFromFloat(3500.50),
		Size:         decimal.NewFromFloat(1.5),
		Timestamp:    time.Now(),
	}
	data, _ := json.Marshal([]models.Trade{trade})
	env := envelope{Type: "Trades", Data: data}
	msg, _ := json.Marshal(env)

	if err := serverConn.WriteMessage(websocket.TextMessage, msg); err != nil {
		t.Fatalf("server write failed: %v", err)
	}

	select {
	case received := <-tradeCh:
		if received.Symbol != "ETH/USDT" {
			t.Errorf("expected symbol ETH/USDT, got %s", received.Symbol)
		}
		if !received.Price.Equal(decimal.NewFromFloat(3500.50)) {
			t.Errorf("expected price 3500.50, got %s", received.Price.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Trade event")
	}
}

// ---------------------------------------------------------------------------
// 4. TestWebSocket_HeartbeatMessage
// ---------------------------------------------------------------------------

func TestWebSocket_HeartbeatMessage(t *testing.T) {
	ready := make(chan *websocket.Conn, 1)

	srv, settings := startMockServer(t, func(conn *websocket.Conn) {
		ready <- conn
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer srv.Close()

	bus, logger := newTestBusAndLogger()
	defer bus.Close()

	_, provCh := bus.Providers.Subscribe(16)

	wsc := New(settings, bus, logger)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := wsc.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer func() { _ = wsc.StopAsync(context.Background()) }()

	// Drain the initial provider events published by StartAsync.
	drainTimeout := time.After(500 * time.Millisecond)
drain:
	for {
		select {
		case <-provCh:
		case <-drainTimeout:
			break drain
		}
	}

	var serverConn *websocket.Conn
	select {
	case serverConn = <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for server connection")
	}

	provider := models.Provider{
		ProviderID:   99,
		ProviderCode: 99,
		ProviderName: "HeartbeatTest",
		Status:       enums.SessionConnected,
		LastUpdated:  time.Now(),
	}
	data, _ := json.Marshal([]models.Provider{provider})
	env := envelope{Type: "HeartBeats", Data: data}
	msg, _ := json.Marshal(env)

	if err := serverConn.WriteMessage(websocket.TextMessage, msg); err != nil {
		t.Fatalf("server write failed: %v", err)
	}

	select {
	case received := <-provCh:
		if received.ProviderID != 99 {
			t.Errorf("expected providerID 99, got %d", received.ProviderID)
		}
		if received.ProviderName != "HeartbeatTest" {
			t.Errorf("expected providerName HeartbeatTest, got %s", received.ProviderName)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Provider heartbeat event")
	}
}

// ---------------------------------------------------------------------------
// 5. TestWebSocket_UnknownType
// ---------------------------------------------------------------------------

func TestWebSocket_UnknownType(t *testing.T) {
	ready := make(chan *websocket.Conn, 1)

	srv, settings := startMockServer(t, func(conn *websocket.Conn) {
		ready <- conn
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer srv.Close()

	bus, logger := newTestBusAndLogger()
	defer bus.Close()

	_, obCh := bus.OrderBooks.Subscribe(16)
	_, tradeCh := bus.Trades.Subscribe(16)

	wsc := New(settings, bus, logger)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := wsc.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer func() { _ = wsc.StopAsync(context.Background()) }()

	var serverConn *websocket.Conn
	select {
	case serverConn = <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for server connection")
	}

	// Send an unknown envelope type.
	env := envelope{Type: "SomethingElse", Data: json.RawMessage(`{}`)}
	msg, _ := json.Marshal(env)

	if err := serverConn.WriteMessage(websocket.TextMessage, msg); err != nil {
		t.Fatalf("server write failed: %v", err)
	}

	// Now send a known type after the unknown one to verify the connector
	// is still alive and processing messages.
	ob := models.OrderBook{Symbol: "ALIVE", ProviderID: 3}
	data, _ := json.Marshal([]models.OrderBook{ob})
	env2 := envelope{Type: "Market", Data: data}
	msg2, _ := json.Marshal(env2)

	if err := serverConn.WriteMessage(websocket.TextMessage, msg2); err != nil {
		t.Fatalf("server write failed: %v", err)
	}

	// The unknown message should NOT produce any event on order books or trades.
	// The subsequent Market message should arrive.
	select {
	case received := <-obCh:
		if received.Symbol != "ALIVE" {
			t.Errorf("expected symbol ALIVE, got %s", received.Symbol)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the follow-up Market message")
	}

	// Ensure no stray trade arrived from the unknown type.
	select {
	case trade := <-tradeCh:
		t.Errorf("unexpected trade received: %+v", trade)
	default:
		// Good: no trade event.
	}
}
