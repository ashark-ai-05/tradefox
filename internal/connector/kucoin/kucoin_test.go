package kucoin

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
// 1. TestKuCoin_BookSnapshot
// ---------------------------------------------------------------------------

func TestKuCoin_BookSnapshot(t *testing.T) {
	logger := slog.Default()
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	_, obCh := bus.OrderBooks.Subscribe(16)

	srv, wsURL := startMockServer(t, func(conn *websocket.Conn) {
		// Send welcome message.
		conn.WriteJSON(map[string]interface{}{
			"id":   "welcome",
			"type": "welcome",
		})

		// Read subscription messages (book + trades).
		for i := 0; i < 2; i++ {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var sub map[string]interface{}
			json.Unmarshal(msg, &sub)
			// Send ack.
			conn.WriteJSON(map[string]interface{}{
				"id":   sub["id"],
				"type": "ack",
			})
		}

		time.Sleep(100 * time.Millisecond)

		// Send L2 update with order book data.
		l2Msg := map[string]interface{}{
			"type":  "message",
			"topic": "/market/level2:BTC-USDT",
			"data": map[string]interface{}{
				"sequenceStart": 100,
				"sequenceEnd":   105,
				"changes": map[string]interface{}{
					"asks": [][]string{
						{"30001", "0.4", "102"},
						{"30002", "0.2", "103"},
					},
					"bids": [][]string{
						{"30000", "0.5", "104"},
						{"29999", "0.3", "105"},
					},
				},
			},
		}
		conn.WriteJSON(l2Msg)

		time.Sleep(200 * time.Millisecond)
	})
	defer srv.Close()

	settings := Settings{
		Symbols:      []string{"BTC-USDT(BTC/USDT)"},
		DepthLevels:  25,
		ProviderID:   4,
		ProviderName: "KuCoin",
		WSURL:        wsURL,
	}

	c := New(settings, bus, logger)

	// Provide a snapshot function that pre-seeds the order book.
	c.SetSnapshotFn(func(symbol string) (*snapshotResponse, error) {
		return &snapshotResponse{
			Sequence: "99",
			Bids: [][]string{
				{"29998", "1.0"},
			},
			Asks: [][]string{
				{"30003", "1.0"},
			},
		}, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer c.StopAsync(ctx)

	// We expect two publications: first from the snapshot, then from the L2 update.
	// Drain the snapshot publication first.
	select {
	case <-obCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for initial snapshot")
	}

	// Now wait for the L2 update.
	select {
	case ob := <-obCh:
		if ob.Symbol != "BTC/USDT" {
			t.Errorf("expected symbol BTC/USDT, got %s", ob.Symbol)
		}
		bids := ob.Bids()
		asks := ob.Asks()
		// Should have bids from snapshot + L2 update.
		if len(bids) < 2 {
			t.Errorf("expected at least 2 bids, got %d", len(bids))
		}
		if len(asks) < 2 {
			t.Errorf("expected at least 2 asks, got %d", len(asks))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for L2 update")
	}
}

// ---------------------------------------------------------------------------
// 2. TestKuCoin_BookUpdateDelete
// ---------------------------------------------------------------------------

func TestKuCoin_BookUpdateDelete(t *testing.T) {
	logger := slog.Default()
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	_, obCh := bus.OrderBooks.Subscribe(16)

	srv, wsURL := startMockServer(t, func(conn *websocket.Conn) {
		conn.WriteJSON(map[string]interface{}{"id": "welcome", "type": "welcome"})

		for i := 0; i < 2; i++ {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var sub map[string]interface{}
			json.Unmarshal(msg, &sub)
			conn.WriteJSON(map[string]interface{}{"id": sub["id"], "type": "ack"})
		}

		time.Sleep(100 * time.Millisecond)

		// Send add.
		conn.WriteJSON(map[string]interface{}{
			"type":  "message",
			"topic": "/market/level2:BTC-USDT",
			"data": map[string]interface{}{
				"sequenceStart": 100,
				"sequenceEnd":   101,
				"changes": map[string]interface{}{
					"asks": [][]string{},
					"bids": [][]string{{"30000", "0.5", "101"}},
				},
			},
		})

		time.Sleep(100 * time.Millisecond)

		// Send delete (size=0).
		conn.WriteJSON(map[string]interface{}{
			"type":  "message",
			"topic": "/market/level2:BTC-USDT",
			"data": map[string]interface{}{
				"sequenceStart": 102,
				"sequenceEnd":   102,
				"changes": map[string]interface{}{
					"asks": [][]string{},
					"bids": [][]string{{"30000", "0", "102"}},
				},
			},
		})

		time.Sleep(200 * time.Millisecond)
	})
	defer srv.Close()

	settings := Settings{
		Symbols:      []string{"BTC-USDT(BTC/USDT)"},
		DepthLevels:  25,
		ProviderID:   4,
		ProviderName: "KuCoin",
		WSURL:        wsURL,
	}

	c := New(settings, bus, logger)

	// Provide a minimal snapshot.
	c.SetSnapshotFn(func(symbol string) (*snapshotResponse, error) {
		return &snapshotResponse{
			Sequence: "99",
			Bids:     [][]string{},
			Asks:     [][]string{},
		}, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer c.StopAsync(ctx)

	// Drain initial empty snapshot publication.
	select {
	case <-obCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for initial snapshot")
	}

	// Receive add.
	select {
	case ob := <-obCh:
		bids := ob.Bids()
		if len(bids) != 1 {
			t.Errorf("expected 1 bid after add, got %d", len(bids))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for add")
	}

	// Receive delete.
	select {
	case ob := <-obCh:
		bids := ob.Bids()
		if len(bids) != 0 {
			t.Errorf("expected 0 bids after delete, got %d", len(bids))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for delete")
	}
}

// ---------------------------------------------------------------------------
// 3. TestKuCoin_TradeMatch
// ---------------------------------------------------------------------------

func TestKuCoin_TradeMatch(t *testing.T) {
	logger := slog.Default()
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	_, tradeCh := bus.Trades.Subscribe(16)

	srv, wsURL := startMockServer(t, func(conn *websocket.Conn) {
		conn.WriteJSON(map[string]interface{}{"id": "welcome", "type": "welcome"})

		for i := 0; i < 2; i++ {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var sub map[string]interface{}
			json.Unmarshal(msg, &sub)
			conn.WriteJSON(map[string]interface{}{"id": sub["id"], "type": "ack"})
		}

		time.Sleep(100 * time.Millisecond)

		// Send trade match.
		conn.WriteJSON(map[string]interface{}{
			"type":  "message",
			"topic": "/market/match:BTC-USDT",
			"data": map[string]interface{}{
				"symbol": "BTC-USDT",
				"price":  "30500",
				"size":   "0.1",
				"side":   "buy",
				"time":   "1704067200000000000",
			},
		})

		time.Sleep(200 * time.Millisecond)
	})
	defer srv.Close()

	settings := Settings{
		Symbols:      []string{"BTC-USDT(BTC/USDT)"},
		DepthLevels:  25,
		ProviderID:   4,
		ProviderName: "KuCoin",
		WSURL:        wsURL,
	}

	c := New(settings, bus, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer c.StopAsync(ctx)

	select {
	case trade := <-tradeCh:
		if trade.Symbol != "BTC/USDT" {
			t.Errorf("expected symbol BTC/USDT, got %s", trade.Symbol)
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
// 4. TestKuCoin_SellTrade
// ---------------------------------------------------------------------------

func TestKuCoin_SellTrade(t *testing.T) {
	logger := slog.Default()
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	_, tradeCh := bus.Trades.Subscribe(16)

	srv, wsURL := startMockServer(t, func(conn *websocket.Conn) {
		conn.WriteJSON(map[string]interface{}{"id": "welcome", "type": "welcome"})

		for i := 0; i < 2; i++ {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var sub map[string]interface{}
			json.Unmarshal(msg, &sub)
			conn.WriteJSON(map[string]interface{}{"id": sub["id"], "type": "ack"})
		}

		time.Sleep(100 * time.Millisecond)

		conn.WriteJSON(map[string]interface{}{
			"type":  "message",
			"topic": "/market/match:BTC-USDT",
			"data": map[string]interface{}{
				"symbol": "BTC-USDT",
				"price":  "30400",
				"size":   "0.25",
				"side":   "sell",
				"time":   "1704067200000000000",
			},
		})

		time.Sleep(200 * time.Millisecond)
	})
	defer srv.Close()

	settings := Settings{
		Symbols:      []string{"BTC-USDT(BTC/USDT)"},
		DepthLevels:  25,
		ProviderID:   4,
		ProviderName: "KuCoin",
		WSURL:        wsURL,
	}

	c := New(settings, bus, logger)

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

// ---------------------------------------------------------------------------
// 5. TestKuCoin_WelcomeAndPong
// ---------------------------------------------------------------------------

func TestKuCoin_WelcomeAndPong(t *testing.T) {
	logger := slog.Default()
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	_, obCh := bus.OrderBooks.Subscribe(16)

	srv, wsURL := startMockServer(t, func(conn *websocket.Conn) {
		// Send welcome.
		conn.WriteJSON(map[string]interface{}{"id": "welcome", "type": "welcome"})

		for i := 0; i < 2; i++ {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var sub map[string]interface{}
			json.Unmarshal(msg, &sub)
			conn.WriteJSON(map[string]interface{}{"id": sub["id"], "type": "ack"})
		}

		time.Sleep(100 * time.Millisecond)

		// Send a pong message (response to ping) - should be ignored.
		conn.WriteJSON(map[string]interface{}{
			"id":   "ping",
			"type": "pong",
		})

		time.Sleep(50 * time.Millisecond)

		// Send actual L2 data.
		conn.WriteJSON(map[string]interface{}{
			"type":  "message",
			"topic": "/market/level2:BTC-USDT",
			"data": map[string]interface{}{
				"sequenceStart": 100,
				"sequenceEnd":   100,
				"changes": map[string]interface{}{
					"asks": [][]string{{"30001", "0.5", "100"}},
					"bids": [][]string{{"30000", "0.5", "100"}},
				},
			},
		})

		time.Sleep(200 * time.Millisecond)
	})
	defer srv.Close()

	settings := Settings{
		Symbols:      []string{"BTC-USDT(BTC/USDT)"},
		DepthLevels:  25,
		ProviderID:   4,
		ProviderName: "KuCoin",
		WSURL:        wsURL,
	}

	c := New(settings, bus, logger)

	// Provide a minimal snapshot with sequence=99.
	c.SetSnapshotFn(func(symbol string) (*snapshotResponse, error) {
		return &snapshotResponse{
			Sequence: "99",
			Bids:     [][]string{},
			Asks:     [][]string{},
		}, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer c.StopAsync(ctx)

	// Drain empty snapshot.
	select {
	case <-obCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for initial snapshot")
	}

	// The pong should be silently ignored; first real event is the L2 data.
	select {
	case ob := <-obCh:
		if ob.Symbol != "BTC/USDT" {
			t.Errorf("expected symbol BTC/USDT, got %s", ob.Symbol)
		}
		bids := ob.Bids()
		if len(bids) != 1 {
			t.Errorf("expected 1 bid, got %d", len(bids))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for L2 data after pong")
	}
}
