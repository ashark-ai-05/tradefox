package bitstamp

import (
	"encoding/json"
	"fmt"
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

// upgrader is used by the mock WebSocket server.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// mockWSServer creates an httptest server that upgrades to WebSocket and calls
// handler with the connection. The returned server must be closed by the caller.
func mockWSServer(t *testing.T, handler func(ws *websocket.Conn)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("upgrade error: %v", err)
			return
		}
		defer ws.Close()
		handler(ws)
	}))
	return srv
}

// wsURL converts an httptest server URL from http:// to ws://.
func wsURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

// mockRESTServer creates an httptest server that serves a static order book
// snapshot JSON response for any path.
func mockRESTServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		snap := snapshotResponse{
			Timestamp: "1234567890",
			Bids: [][]string{
				{"30000.00", "1.5"},
				{"29999.00", "2.0"},
			},
			Asks: [][]string{
				{"30001.00", "1.0"},
				{"30002.00", "0.5"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(snap)
	}))
}

// newTestConnector creates a BitStampConnector wired to mock servers and a
// fresh event bus.
func newTestConnector(t *testing.T, wsHandler func(ws *websocket.Conn)) (*BitStampConnector, *eventbus.Bus, *httptest.Server, *httptest.Server) {
	t.Helper()

	wsSrv := mockWSServer(t, wsHandler)
	restSrv := mockRESTServer(t)

	logger := slog.Default()
	bus := eventbus.NewBus(logger)

	settings := Settings{
		HostName:          restSrv.URL,
		WebSocketHostName: wsURL(wsSrv),
		Symbols:           []string{"btcusd(BTC/USD)"},
		DepthLevels:       10,
		ProviderID:        6,
		ProviderName:      "BitStamp",
	}

	c := New(settings, bus, logger)
	return c, bus, wsSrv, restSrv
}

// ---------------------------------------------------------------------------
// 1. TestBitStamp_Settings_Defaults
// ---------------------------------------------------------------------------

func TestBitStamp_Settings_Defaults(t *testing.T) {
	s := DefaultSettings()

	if s.HostName != "https://www.bitstamp.net/api/v2/" {
		t.Errorf("expected default HostName, got %s", s.HostName)
	}
	if s.WebSocketHostName != "wss://ws.bitstamp.net" {
		t.Errorf("expected default WebSocketHostName, got %s", s.WebSocketHostName)
	}
	if s.DepthLevels != 10 {
		t.Errorf("expected DepthLevels 10, got %d", s.DepthLevels)
	}
	if s.ProviderID != 6 {
		t.Errorf("expected ProviderID 6, got %d", s.ProviderID)
	}
	if s.ProviderName != "BitStamp" {
		t.Errorf("expected ProviderName BitStamp, got %s", s.ProviderName)
	}
	if len(s.Symbols) != 1 || s.Symbols[0] != "btcusd(BTC/USD)" {
		t.Errorf("expected default symbols, got %v", s.Symbols)
	}
}

// ---------------------------------------------------------------------------
// 2. TestBitStamp_Subscribe
// ---------------------------------------------------------------------------

func TestBitStamp_Subscribe(t *testing.T) {
	var receivedMsgs []wsMessage
	var mu sync.Mutex
	done := make(chan struct{})

	wsHandler := func(ws *websocket.Conn) {
		for {
			_, raw, err := ws.ReadMessage()
			if err != nil {
				return
			}
			var msg wsMessage
			if err := json.Unmarshal(raw, &msg); err != nil {
				continue
			}
			mu.Lock()
			receivedMsgs = append(receivedMsgs, msg)
			if len(receivedMsgs) >= 2 {
				select {
				case <-done:
				default:
					close(done)
				}
			}
			mu.Unlock()
		}
	}

	wsSrv := mockWSServer(t, wsHandler)
	defer wsSrv.Close()

	restSrv := mockRESTServer(t)
	defer restSrv.Close()

	logger := slog.Default()
	bus := eventbus.NewBus(logger)

	settings := Settings{
		HostName:          restSrv.URL,
		WebSocketHostName: wsURL(wsSrv),
		Symbols:           []string{"btcusd(BTC/USD)"},
		DepthLevels:       10,
		ProviderID:        6,
		ProviderName:      "BitStamp",
	}

	c := New(settings, bus, logger)

	ctx := t.Context()
	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync error: %v", err)
	}
	defer c.StopAsync(ctx)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for subscription messages")
	}

	mu.Lock()
	defer mu.Unlock()

	if len(receivedMsgs) < 2 {
		t.Fatalf("expected at least 2 subscription messages, got %d", len(receivedMsgs))
	}

	// Verify subscription events.
	for _, msg := range receivedMsgs {
		if msg.Event != "bts:subscribe" {
			t.Errorf("expected event bts:subscribe, got %s", msg.Event)
		}
	}

	// Extract channels from the data payloads.
	channels := make(map[string]bool)
	for _, msg := range receivedMsgs {
		var data struct {
			Channel string `json:"channel"`
		}
		if err := json.Unmarshal(msg.Data, &data); err == nil {
			channels[data.Channel] = true
		}
	}

	if !channels["diff_order_book_btcusd"] {
		t.Error("expected subscription to diff_order_book_btcusd")
	}
	if !channels["live_trades_btcusd"] {
		t.Error("expected subscription to live_trades_btcusd")
	}
}

// ---------------------------------------------------------------------------
// 3. TestBitStamp_DeltaUpdate
// ---------------------------------------------------------------------------

func TestBitStamp_DeltaUpdate(t *testing.T) {
	ready := make(chan *websocket.Conn, 1)

	wsHandler := func(ws *websocket.Conn) {
		// Drain subscription messages.
		for i := 0; i < 2; i++ {
			ws.ReadMessage()
		}
		ready <- ws
		// Keep connection alive.
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
			}
		}
	}

	c, bus, wsSrv, restSrv := newTestConnector(t, wsHandler)
	defer wsSrv.Close()
	defer restSrv.Close()

	_, obCh := bus.OrderBooks.Subscribe(16)

	ctx := t.Context()
	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync error: %v", err)
	}
	defer c.StopAsync(ctx)

	// Drain the initial snapshot publish.
	select {
	case <-obCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for initial snapshot")
	}

	// Wait for the WS handler to be ready.
	var ws *websocket.Conn
	select {
	case ws = <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for WS handler ready")
	}

	// Send delta update: update bid at 30000.00 to size 3.0, add ask at 30003.00.
	delta := fmt.Sprintf(`{
		"event": "data",
		"channel": "diff_order_book_btcusd",
		"data": {
			"timestamp": "1234567890",
			"microtimestamp": "1234567890000000",
			"bids": [["30000.00", "3.0"]],
			"asks": [["30003.00", "0.8"]]
		}
	}`)
	if err := ws.WriteMessage(websocket.TextMessage, []byte(delta)); err != nil {
		t.Fatalf("write delta: %v", err)
	}

	// Wait for the delta-triggered order book publish.
	select {
	case ob := <-obCh:
		if ob.Symbol != "BTC/USD" {
			t.Errorf("expected symbol BTC/USD, got %s", ob.Symbol)
		}
		// Verify the bid at 30000.00 was updated to size 3.0.
		bids := ob.Bids()
		found := false
		for _, b := range bids {
			if b.Price != nil && *b.Price == 30000.00 && b.Size != nil && *b.Size == 3.0 {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected bid at 30000.00 with size 3.0 not found")
		}

		// Verify the ask at 30003.00 was added.
		asks := ob.Asks()
		found = false
		for _, a := range asks {
			if a.Price != nil && *a.Price == 30003.00 && a.Size != nil && *a.Size == 0.8 {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected ask at 30003.00 with size 0.8 not found")
		}

	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for delta order book update")
	}
}

// ---------------------------------------------------------------------------
// 4. TestBitStamp_Trade
// ---------------------------------------------------------------------------

func TestBitStamp_Trade(t *testing.T) {
	ready := make(chan *websocket.Conn, 1)

	wsHandler := func(ws *websocket.Conn) {
		// Drain subscription messages.
		for i := 0; i < 2; i++ {
			ws.ReadMessage()
		}
		ready <- ws
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
			}
		}
	}

	c, bus, wsSrv, restSrv := newTestConnector(t, wsHandler)
	defer wsSrv.Close()
	defer restSrv.Close()

	_, tradeCh := bus.Trades.Subscribe(16)

	ctx := t.Context()
	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync error: %v", err)
	}
	defer c.StopAsync(ctx)

	var ws *websocket.Conn
	select {
	case ws = <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for WS handler ready")
	}

	// Send a trade message (type=0 is buy).
	tradeMsg := `{
		"event": "trade",
		"channel": "live_trades_btcusd",
		"data": {
			"id": 12345,
			"price": "30000.50",
			"amount": "0.25",
			"type": 0,
			"microtimestamp": "1234567890000000"
		}
	}`
	if err := ws.WriteMessage(websocket.TextMessage, []byte(tradeMsg)); err != nil {
		t.Fatalf("write trade: %v", err)
	}

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
			t.Error("expected IsBuy=true for type=0")
		}
		if trade.ProviderID != 6 {
			t.Errorf("expected ProviderID 6, got %d", trade.ProviderID)
		}
		if trade.ProviderName != "BitStamp" {
			t.Errorf("expected ProviderName BitStamp, got %s", trade.ProviderName)
		}

	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for trade event")
	}

	// Send a sell trade (type=1).
	sellMsg := `{
		"event": "trade",
		"channel": "live_trades_btcusd",
		"data": {
			"id": 12346,
			"price": "29999.00",
			"amount": "1.0",
			"type": 1,
			"microtimestamp": "1234567891000000"
		}
	}`
	if err := ws.WriteMessage(websocket.TextMessage, []byte(sellMsg)); err != nil {
		t.Fatalf("write sell trade: %v", err)
	}

	select {
	case trade := <-tradeCh:
		if trade.IsBuy == nil || *trade.IsBuy {
			t.Error("expected IsBuy=false for type=1")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for sell trade event")
	}
}

// ---------------------------------------------------------------------------
// 5. TestBitStamp_DeleteLevel
// ---------------------------------------------------------------------------

func TestBitStamp_DeleteLevel(t *testing.T) {
	ready := make(chan *websocket.Conn, 1)

	wsHandler := func(ws *websocket.Conn) {
		for i := 0; i < 2; i++ {
			ws.ReadMessage()
		}
		ready <- ws
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
			}
		}
	}

	c, bus, wsSrv, restSrv := newTestConnector(t, wsHandler)
	defer wsSrv.Close()
	defer restSrv.Close()

	_, obCh := bus.OrderBooks.Subscribe(16)

	ctx := t.Context()
	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync error: %v", err)
	}
	defer c.StopAsync(ctx)

	// Drain the initial snapshot.
	select {
	case <-obCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for initial snapshot")
	}

	var ws *websocket.Conn
	select {
	case ws = <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for WS handler ready")
	}

	// Send delta with qty=0 for bid at 30000.00 (delete that level).
	deleteMsg := `{
		"event": "data",
		"channel": "diff_order_book_btcusd",
		"data": {
			"timestamp": "1234567890",
			"microtimestamp": "1234567890000000",
			"bids": [["30000.00", "0"]],
			"asks": []
		}
	}`
	if err := ws.WriteMessage(websocket.TextMessage, []byte(deleteMsg)); err != nil {
		t.Fatalf("write delete delta: %v", err)
	}

	select {
	case ob := <-obCh:
		bids := ob.Bids()
		for _, b := range bids {
			if b.Price != nil && *b.Price == 30000.00 {
				t.Error("expected bid at 30000.00 to be deleted, but it still exists")
			}
		}
		// Verify the other bid level (29999.00) still exists.
		found := false
		for _, b := range bids {
			if b.Price != nil && *b.Price == 29999.00 {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected bid at 29999.00 to remain")
		}

	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for delete order book update")
	}
}

// ---------------------------------------------------------------------------
// 6. TestBitStamp_Heartbeat
// ---------------------------------------------------------------------------

func TestBitStamp_Heartbeat(t *testing.T) {
	ready := make(chan *websocket.Conn, 1)

	wsHandler := func(ws *websocket.Conn) {
		for i := 0; i < 2; i++ {
			ws.ReadMessage()
		}
		ready <- ws
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
			}
		}
	}

	c, bus, wsSrv, restSrv := newTestConnector(t, wsHandler)
	defer wsSrv.Close()
	defer restSrv.Close()

	_, provCh := bus.Providers.Subscribe(16)

	ctx := t.Context()
	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync error: %v", err)
	}
	defer c.StopAsync(ctx)

	// Drain the initial provider event(s) from StartAsync/connect.
	drainTimeout := time.After(3 * time.Second)
drainLoop:
	for {
		select {
		case <-provCh:
		case <-drainTimeout:
			break drainLoop
		case <-time.After(500 * time.Millisecond):
			break drainLoop
		}
	}

	var ws *websocket.Conn
	select {
	case ws = <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for WS handler ready")
	}

	// Send heartbeat.
	heartbeat := `{"event": "bts:heartbeat"}`
	if err := ws.WriteMessage(websocket.TextMessage, []byte(heartbeat)); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}

	select {
	case prov := <-provCh:
		if prov.Status != enums.SessionConnected {
			t.Errorf("expected SessionConnected after heartbeat, got %v", prov.Status)
		}
		if prov.ProviderID != 6 {
			t.Errorf("expected ProviderID 6, got %d", prov.ProviderID)
		}
		if prov.ProviderName != "BitStamp" {
			t.Errorf("expected ProviderName BitStamp, got %s", prov.ProviderName)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for heartbeat provider event")
	}
}
