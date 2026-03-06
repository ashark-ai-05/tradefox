package bitfinex

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
// 1. TestBitfinex_BookSnapshot
// ---------------------------------------------------------------------------

func TestBitfinex_BookSnapshot(t *testing.T) {
	logger := slog.Default()
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	_, obCh := bus.OrderBooks.Subscribe(16)

	srv, wsURL := startMockServer(t, func(conn *websocket.Conn) {
		// Read subscription messages (book + trades for one symbol).
		for i := 0; i < 2; i++ {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var sub map[string]interface{}
			json.Unmarshal(msg, &sub)

			channel := sub["channel"].(string)
			chanID := 10 + i

			// Send subscribed event.
			resp := map[string]interface{}{
				"event":   "subscribed",
				"channel": channel,
				"chanId":  chanID,
				"symbol":  "tBTCUSD",
			}
			conn.WriteJSON(resp)
		}

		// Give client time to register channels.
		time.Sleep(50 * time.Millisecond)

		// Send book snapshot: chanID=10 (book).
		// [10, [[30000, 2, 0.5], [29999, 1, 0.3], [30001, 3, -0.4], [30002, 1, -0.2]]]
		snapshot := []interface{}{
			10,
			[][]float64{
				{30000, 2, 0.5},  // bid (positive amount)
				{29999, 1, 0.3},  // bid
				{30001, 3, -0.4}, // ask (negative amount)
				{30002, 1, -0.2}, // ask
			},
		}
		conn.WriteJSON(snapshot)

		// Keep connection alive briefly.
		time.Sleep(200 * time.Millisecond)
	})
	defer srv.Close()

	settings := Settings{
		Symbols:      []string{"BTCUSD(BTC/USD)"},
		DepthLevels:  25,
		ProviderID:   2,
		ProviderName: "Bitfinex",
	}

	c := New(settings, bus, logger)
	c.SetWSURL(wsURL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer c.StopAsync(ctx)

	// Wait for order book publication.
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
		// Best bid should be 30000.
		if len(bids) > 0 && *bids[0].Price != 30000 {
			t.Errorf("expected best bid 30000, got %f", *bids[0].Price)
		}
		// Best ask should be 30001.
		if len(asks) > 0 && *asks[0].Price != 30001 {
			t.Errorf("expected best ask 30001, got %f", *asks[0].Price)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for order book snapshot")
	}
}

// ---------------------------------------------------------------------------
// 2. TestBitfinex_BookUpdate
// ---------------------------------------------------------------------------

func TestBitfinex_BookUpdate(t *testing.T) {
	logger := slog.Default()
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	_, obCh := bus.OrderBooks.Subscribe(16)

	srv, wsURL := startMockServer(t, func(conn *websocket.Conn) {
		// Read subscription messages.
		for i := 0; i < 2; i++ {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var sub map[string]interface{}
			json.Unmarshal(msg, &sub)
			channel := sub["channel"].(string)
			resp := map[string]interface{}{
				"event":   "subscribed",
				"channel": channel,
				"chanId":  10 + i,
				"symbol":  "tBTCUSD",
			}
			conn.WriteJSON(resp)
		}

		time.Sleep(50 * time.Millisecond)

		// Send snapshot first.
		snapshot := []interface{}{
			10,
			[][]float64{
				{30000, 2, 0.5},
				{30001, 3, -0.4},
			},
		}
		conn.WriteJSON(snapshot)

		time.Sleep(100 * time.Millisecond)

		// Send update: delete the bid at 30000 (count=0, positive amount means bid side).
		update := []interface{}{10, []float64{30000, 0, 1.0}}
		conn.WriteJSON(update)

		time.Sleep(200 * time.Millisecond)
	})
	defer srv.Close()

	settings := Settings{
		Symbols:      []string{"BTCUSD(BTC/USD)"},
		DepthLevels:  25,
		ProviderID:   2,
		ProviderName: "Bitfinex",
	}

	c := New(settings, bus, logger)
	c.SetWSURL(wsURL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer c.StopAsync(ctx)

	// First publication is the snapshot.
	select {
	case <-obCh:
		// Snapshot received; wait for the update.
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for snapshot")
	}

	// Second publication is the update (delete).
	select {
	case ob := <-obCh:
		bids := ob.Bids()
		if len(bids) != 0 {
			t.Errorf("expected 0 bids after delete, got %d", len(bids))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for update")
	}
}

// ---------------------------------------------------------------------------
// 3. TestBitfinex_TradeExecution
// ---------------------------------------------------------------------------

func TestBitfinex_TradeExecution(t *testing.T) {
	logger := slog.Default()
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	_, tradeCh := bus.Trades.Subscribe(16)

	srv, wsURL := startMockServer(t, func(conn *websocket.Conn) {
		// Read subscription messages.
		for i := 0; i < 2; i++ {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var sub map[string]interface{}
			json.Unmarshal(msg, &sub)
			channel := sub["channel"].(string)
			chanID := 10
			if channel == "trades" {
				chanID = 11
			}
			resp := map[string]interface{}{
				"event":   "subscribed",
				"channel": channel,
				"chanId":  chanID,
				"symbol":  "tBTCUSD",
			}
			conn.WriteJSON(resp)
		}

		time.Sleep(50 * time.Millisecond)

		// Send trade execution: [11, "te", [12345, 1704067200000, 0.1, 30500]]
		// Positive amount = buy.
		trade := []interface{}{11, "te", []float64{12345, 1704067200000, 0.1, 30500}}
		conn.WriteJSON(trade)

		time.Sleep(200 * time.Millisecond)
	})
	defer srv.Close()

	settings := Settings{
		Symbols:      []string{"BTCUSD(BTC/USD)"},
		DepthLevels:  25,
		ProviderID:   2,
		ProviderName: "Bitfinex",
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
			t.Error("expected IsBuy=true for positive amount")
		}
		priceFloat, _ := trade.Price.Float64()
		if priceFloat != 30500 {
			t.Errorf("expected price 30500, got %f", priceFloat)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for trade")
	}
}

// ---------------------------------------------------------------------------
// 4. TestBitfinex_Heartbeat
// ---------------------------------------------------------------------------

func TestBitfinex_Heartbeat(t *testing.T) {
	logger := slog.Default()
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	_, obCh := bus.OrderBooks.Subscribe(16)

	srv, wsURL := startMockServer(t, func(conn *websocket.Conn) {
		// Read subscription messages.
		for i := 0; i < 2; i++ {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var sub map[string]interface{}
			json.Unmarshal(msg, &sub)
			channel := sub["channel"].(string)
			resp := map[string]interface{}{
				"event":   "subscribed",
				"channel": channel,
				"chanId":  10 + i,
				"symbol":  "tBTCUSD",
			}
			conn.WriteJSON(resp)
		}

		time.Sleep(50 * time.Millisecond)

		// Send heartbeat - should be silently ignored.
		hb := []interface{}{10, "hb"}
		conn.WriteJSON(hb)

		time.Sleep(50 * time.Millisecond)

		// Then send an actual snapshot.
		snapshot := []interface{}{
			10,
			[][]float64{
				{30000, 2, 0.5},
				{30001, 3, -0.4},
			},
		}
		conn.WriteJSON(snapshot)

		time.Sleep(200 * time.Millisecond)
	})
	defer srv.Close()

	settings := Settings{
		Symbols:      []string{"BTCUSD(BTC/USD)"},
		DepthLevels:  25,
		ProviderID:   2,
		ProviderName: "Bitfinex",
	}

	c := New(settings, bus, logger)
	c.SetWSURL(wsURL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer c.StopAsync(ctx)

	// The heartbeat should not produce an order book event. The first event
	// should be the snapshot.
	select {
	case ob := <-obCh:
		if ob.Symbol != "BTC/USD" {
			t.Errorf("expected symbol BTC/USD, got %s", ob.Symbol)
		}
		bids := ob.Bids()
		if len(bids) != 1 {
			t.Errorf("expected 1 bid, got %d", len(bids))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for order book after heartbeat")
	}
}

// ---------------------------------------------------------------------------
// 5. TestBitfinex_SellTrade
// ---------------------------------------------------------------------------

func TestBitfinex_SellTrade(t *testing.T) {
	logger := slog.Default()
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	_, tradeCh := bus.Trades.Subscribe(16)

	srv, wsURL := startMockServer(t, func(conn *websocket.Conn) {
		for i := 0; i < 2; i++ {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var sub map[string]interface{}
			json.Unmarshal(msg, &sub)
			channel := sub["channel"].(string)
			chanID := 10
			if channel == "trades" {
				chanID = 11
			}
			resp := map[string]interface{}{
				"event":   "subscribed",
				"channel": channel,
				"chanId":  chanID,
				"symbol":  "tBTCUSD",
			}
			conn.WriteJSON(resp)
		}

		time.Sleep(50 * time.Millisecond)

		// Send sell trade: negative amount.
		trade := []interface{}{11, "te", []float64{12346, 1704067200000, -0.25, 30400}}
		conn.WriteJSON(trade)

		time.Sleep(200 * time.Millisecond)
	})
	defer srv.Close()

	settings := Settings{
		Symbols:      []string{"BTCUSD(BTC/USD)"},
		DepthLevels:  25,
		ProviderID:   2,
		ProviderName: "Bitfinex",
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
			t.Error("expected IsBuy=false for negative amount (sell trade)")
		}
		sizeFloat, _ := trade.Size.Float64()
		if sizeFloat != 0.25 {
			t.Errorf("expected size 0.25, got %f", sizeFloat)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for sell trade")
	}
}
