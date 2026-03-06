package kraken

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
	"github.com/ashark-ai-05/tradefox/internal/core/interfaces"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

// upgrader is the WebSocket upgrader used by mock servers.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// startMockServer creates a mock WebSocket server that calls handler for each
// connection. Returns the server and its ws:// URL.
func startMockServer(t *testing.T, handler func(conn *websocket.Conn)) (*httptest.Server, string) {
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

// Compile-time check: Connector satisfies interfaces.Connector.
var _ interfaces.Connector = (*Connector)(nil)

// ---------------------------------------------------------------------------
// 1. TestKraken_BookSnapshot
// ---------------------------------------------------------------------------

func TestKraken_BookSnapshot(t *testing.T) {
	logger := slog.Default()
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	_, obCh := bus.OrderBooks.Subscribe(16)

	srv, wsURL := startMockServer(t, func(conn *websocket.Conn) {
		// Read two subscription messages (book + trade).
		for i := 0; i < 2; i++ {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}

		time.Sleep(50 * time.Millisecond)

		// Send book snapshot.
		snapshot := map[string]interface{}{
			"channel": "book",
			"type":    "snapshot",
			"data": []map[string]interface{}{
				{
					"symbol": "BTC/USD",
					"bids": []map[string]interface{}{
						{"price": 30000.0, "qty": 0.5},
						{"price": 29999.0, "qty": 0.3},
					},
					"asks": []map[string]interface{}{
						{"price": 30001.0, "qty": 0.4},
						{"price": 30002.0, "qty": 0.2},
					},
				},
			},
		}
		conn.WriteJSON(snapshot)

		time.Sleep(200 * time.Millisecond)
	})
	defer srv.Close()

	settings := Settings{
		Symbols:      []string{"BTC/USD"},
		DepthLevels:  25,
		ProviderID:   3,
		ProviderName: "Kraken",
	}

	c := New(settings, bus, logger)
	c.SetWSURL(wsURL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer c.StopAsync(ctx)

	select {
	case ob := <-obCh:
		if ob.Symbol != "BTC/USD" {
			t.Errorf("expected symbol BTC/USD, got %s", ob.Symbol)
		}
		bids := ob.Bids()
		asks := ob.Asks()
		if len(bids) != 2 {
			t.Errorf("expected 2 bids, got %d", len(bids))
		}
		if len(asks) != 2 {
			t.Errorf("expected 2 asks, got %d", len(asks))
		}
		if len(bids) > 0 && *bids[0].Price != 30000 {
			t.Errorf("expected best bid 30000, got %f", *bids[0].Price)
		}
		if len(asks) > 0 && *asks[0].Price != 30001 {
			t.Errorf("expected best ask 30001, got %f", *asks[0].Price)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for order book snapshot")
	}
}

// ---------------------------------------------------------------------------
// 2. TestKraken_BookUpdate
// ---------------------------------------------------------------------------

func TestKraken_BookUpdate(t *testing.T) {
	logger := slog.Default()
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	_, obCh := bus.OrderBooks.Subscribe(16)

	srv, wsURL := startMockServer(t, func(conn *websocket.Conn) {
		for i := 0; i < 2; i++ {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}

		time.Sleep(50 * time.Millisecond)

		// Send snapshot.
		snapshot := map[string]interface{}{
			"channel": "book",
			"type":    "snapshot",
			"data": []map[string]interface{}{
				{
					"symbol": "BTC/USD",
					"bids":   []map[string]interface{}{{"price": 30000.0, "qty": 0.5}},
					"asks":   []map[string]interface{}{{"price": 30001.0, "qty": 0.4}},
				},
			},
		}
		conn.WriteJSON(snapshot)

		time.Sleep(100 * time.Millisecond)

		// Send update: delete bid at 30000 (qty=0) and add new ask.
		update := map[string]interface{}{
			"channel": "book",
			"type":    "update",
			"data": []map[string]interface{}{
				{
					"symbol":   "BTC/USD",
					"bids":     []map[string]interface{}{{"price": 30000.0, "qty": 0.0}},
					"asks":     []map[string]interface{}{{"price": 30003.0, "qty": 0.1}},
					"checksum": 12345,
				},
			},
		}
		conn.WriteJSON(update)

		time.Sleep(200 * time.Millisecond)
	})
	defer srv.Close()

	settings := Settings{
		Symbols:      []string{"BTC/USD"},
		DepthLevels:  25,
		ProviderID:   3,
		ProviderName: "Kraken",
	}

	c := New(settings, bus, logger)
	c.SetWSURL(wsURL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer c.StopAsync(ctx)

	// Receive snapshot.
	select {
	case <-obCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for snapshot")
	}

	// Receive update.
	select {
	case ob := <-obCh:
		bids := ob.Bids()
		asks := ob.Asks()
		if len(bids) != 0 {
			t.Errorf("expected 0 bids after delete, got %d", len(bids))
		}
		if len(asks) != 2 {
			t.Errorf("expected 2 asks after add, got %d", len(asks))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for update")
	}
}

// ---------------------------------------------------------------------------
// 3. TestKraken_TradeUpdate
// ---------------------------------------------------------------------------

func TestKraken_TradeUpdate(t *testing.T) {
	logger := slog.Default()
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	_, tradeCh := bus.Trades.Subscribe(16)

	srv, wsURL := startMockServer(t, func(conn *websocket.Conn) {
		for i := 0; i < 2; i++ {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}

		time.Sleep(50 * time.Millisecond)

		// Send trade update.
		tradeMsg := map[string]interface{}{
			"channel": "trade",
			"type":    "update",
			"data": []map[string]interface{}{
				{
					"symbol":    "BTC/USD",
					"price":     30500.0,
					"qty":       0.1,
					"side":      "buy",
					"timestamp": "2024-01-01T00:00:00.000Z",
				},
			},
		}
		conn.WriteJSON(tradeMsg)

		time.Sleep(200 * time.Millisecond)
	})
	defer srv.Close()

	settings := Settings{
		Symbols:      []string{"BTC/USD"},
		DepthLevels:  25,
		ProviderID:   3,
		ProviderName: "Kraken",
	}

	c := New(settings, bus, logger)
	c.SetWSURL(wsURL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer c.StopAsync(ctx)

	select {
	case trade := <-tradeCh:
		if trade.Symbol != "BTC/USD" {
			t.Errorf("expected symbol BTC/USD, got %s", trade.Symbol)
		}
		if trade.IsBuy == nil || !*trade.IsBuy {
			t.Error("expected IsBuy=true")
		}
		priceFloat, _ := trade.Price.Float64()
		if priceFloat != 30500 {
			t.Errorf("expected price 30500, got %f", priceFloat)
		}
		sizeFloat, _ := trade.Size.Float64()
		if sizeFloat != 0.1 {
			t.Errorf("expected size 0.1, got %f", sizeFloat)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for trade")
	}
}

// ---------------------------------------------------------------------------
// 4. TestKraken_HeartbeatIgnored
// ---------------------------------------------------------------------------

func TestKraken_HeartbeatIgnored(t *testing.T) {
	logger := slog.Default()
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	_, obCh := bus.OrderBooks.Subscribe(16)

	srv, wsURL := startMockServer(t, func(conn *websocket.Conn) {
		for i := 0; i < 2; i++ {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}

		time.Sleep(50 * time.Millisecond)

		// Send heartbeat - should be ignored.
		conn.WriteJSON(map[string]interface{}{
			"channel": "heartbeat",
		})

		// Send status - should be ignored.
		conn.WriteJSON(map[string]interface{}{
			"channel": "status",
			"type":    "update",
			"data":    json.RawMessage(`[{"system":"online"}]`),
		})

		time.Sleep(50 * time.Millisecond)

		// Now send a real book snapshot.
		snapshot := map[string]interface{}{
			"channel": "book",
			"type":    "snapshot",
			"data": []map[string]interface{}{
				{
					"symbol": "BTC/USD",
					"bids":   []map[string]interface{}{{"price": 30000.0, "qty": 0.5}},
					"asks":   []map[string]interface{}{{"price": 30001.0, "qty": 0.4}},
				},
			},
		}
		conn.WriteJSON(snapshot)

		time.Sleep(200 * time.Millisecond)
	})
	defer srv.Close()

	settings := Settings{
		Symbols:      []string{"BTC/USD"},
		DepthLevels:  25,
		ProviderID:   3,
		ProviderName: "Kraken",
	}

	c := New(settings, bus, logger)
	c.SetWSURL(wsURL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer c.StopAsync(ctx)

	// First event should be the book snapshot (heartbeats are ignored).
	select {
	case ob := <-obCh:
		if ob.Symbol != "BTC/USD" {
			t.Errorf("expected symbol BTC/USD, got %s", ob.Symbol)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for snapshot after heartbeat")
	}
}

// ---------------------------------------------------------------------------
// 5. TestKraken_SellTrade
// ---------------------------------------------------------------------------

func TestKraken_SellTrade(t *testing.T) {
	logger := slog.Default()
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	_, tradeCh := bus.Trades.Subscribe(16)

	srv, wsURL := startMockServer(t, func(conn *websocket.Conn) {
		for i := 0; i < 2; i++ {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}

		time.Sleep(50 * time.Millisecond)

		tradeMsg := map[string]interface{}{
			"channel": "trade",
			"type":    "update",
			"data": []map[string]interface{}{
				{
					"symbol":    "BTC/USD",
					"price":     30400.0,
					"qty":       0.25,
					"side":      "sell",
					"timestamp": "2024-01-01T12:00:00.000Z",
				},
			},
		}
		conn.WriteJSON(tradeMsg)

		time.Sleep(200 * time.Millisecond)
	})
	defer srv.Close()

	settings := Settings{
		Symbols:      []string{"BTC/USD"},
		DepthLevels:  25,
		ProviderID:   3,
		ProviderName: "Kraken",
	}

	c := New(settings, bus, logger)
	c.SetWSURL(wsURL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer c.StopAsync(ctx)

	select {
	case trade := <-tradeCh:
		if trade.IsBuy == nil || *trade.IsBuy {
			t.Error("expected IsBuy=false for sell trade")
		}
		sizeFloat, _ := trade.Size.Float64()
		if sizeFloat != 0.25 {
			t.Errorf("expected size 0.25, got %f", sizeFloat)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for sell trade")
	}
}
