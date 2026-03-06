package binance

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

	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// mockWSServer creates an httptest server that upgrades HTTP connections to
// WebSocket and invokes handler for each connection. Returns the server and a
// ws:// URL suitable for dialling.
func mockWSServer(t *testing.T, handler func(conn *websocket.Conn)) (*httptest.Server, string) {
	t.Helper()
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

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

// mockRESTServer creates an httptest server that serves a fixed snapshot
// response at /api/v3/depth.
func mockRESTServer(t *testing.T, snapshot snapshotResponse) *httptest.Server {
	t.Helper()
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))
}

// newTestBus creates an event bus with a default logger.
func newTestBus() *eventbus.Bus {
	return eventbus.NewBus(slog.Default())
}

// newTestConnector creates a BinanceConnector with overridden URLs for testing.
// It returns the connector plus the REST server (caller must close).
func newTestConnector(t *testing.T, wsURL string, snapshot snapshotResponse, symbols []string) (*BinanceConnector, *httptest.Server) {
	t.Helper()

	restSrv := mockRESTServer(t, snapshot)

	bus := newTestBus()
	c := New(bus, nil, slog.Default())
	c.settings.Symbols = symbols
	c.wsURL = wsURL
	c.restURL = restSrv.URL
	c.httpClient = restSrv.Client()

	return c, restSrv
}

// waitForCondition polls fn every 10ms up to timeout and returns true if fn
// returns true within that window.
func waitForCondition(timeout time.Duration, fn func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestBinance_Settings_Defaults(t *testing.T) {
	s := DefaultSettings()

	if s.DepthLevels != 10 {
		t.Errorf("expected DepthLevels=10, got %d", s.DepthLevels)
	}
	if s.UpdateIntervalMs != 100 {
		t.Errorf("expected UpdateIntervalMs=100, got %d", s.UpdateIntervalMs)
	}
	if !s.IsNonUS {
		t.Error("expected IsNonUS=true")
	}
	if s.ProviderID != 1 {
		t.Errorf("expected ProviderID=1, got %d", s.ProviderID)
	}
	if s.ProviderName != "Binance" {
		t.Errorf("expected ProviderName=Binance, got %s", s.ProviderName)
	}
	if s.ApiKey != "" {
		t.Errorf("expected ApiKey to be empty, got %q", s.ApiKey)
	}
}

func TestBinance_ParseSymbols(t *testing.T) {
	bus := newTestBus()
	c := New(bus, nil, slog.Default())

	c.settings.Symbols = []string{"BTCUSDT(BTC/USDT)", "ETHUSDT"}
	symStr := strings.Join(c.settings.Symbols, ",")
	c.ParseSymbols(symStr)

	if got := c.GetNormalizedSymbol("BTCUSDT"); got != "BTC/USDT" {
		t.Errorf("expected BTC/USDT, got %s", got)
	}
	if got := c.GetNormalizedSymbol("ETHUSDT"); got != "ETHUSDT" {
		t.Errorf("expected ETHUSDT, got %s", got)
	}

	allExchange := c.GetAllExchangeSymbols()
	if len(allExchange) != 2 {
		t.Errorf("expected 2 exchange symbols, got %d", len(allExchange))
	}
}

func TestBinance_DepthUpdate(t *testing.T) {
	snapshot := snapshotResponse{
		LastUpdateID: 100,
		Bids:         [][]string{{"30000.00", "1.5"}, {"29999.00", "2.0"}},
		Asks:         [][]string{{"30001.00", "1.0"}, {"30002.00", "0.5"}},
	}

	depthUpdate := depthMsg{
		EventType:     "depthUpdate",
		EventTime:     time.Now().UnixMilli(),
		Symbol:        "BTCUSDT",
		FirstUpdateID: 101,
		FinalUpdateID: 102,
		Bids:          [][]string{{"30000.00", "2.0"}},
		Asks:          [][]string{{"30001.00", "0.8"}},
	}

	depthData, _ := json.Marshal(depthUpdate)
	combined := combinedMsg{
		Stream: "btcusdt@depth@100ms",
		Data:   depthData,
	}
	combinedData, _ := json.Marshal(combined)

	// WS server: send one depth update then block until closed.
	wsSrv, wsURL := mockWSServer(t, func(conn *websocket.Conn) {
		_ = conn.WriteMessage(websocket.TextMessage, combinedData)
		// Keep connection open until test completes.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer wsSrv.Close()

	c, restSrv := newTestConnector(t, wsURL, snapshot, []string{"BTCUSDT"})
	defer restSrv.Close()

	// Subscribe to order books.
	_, obCh := c.bus.OrderBooks.Subscribe(16)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer func() { _ = c.StopAsync(context.Background()) }()

	// Wait for at least one order book publication (snapshot + delta).
	var lastOB *models.OrderBook
	ok := waitForCondition(3*time.Second, func() bool {
		select {
		case ob := <-obCh:
			lastOB = ob
			return true
		default:
			return false
		}
	})
	if !ok {
		t.Fatal("timed out waiting for order book publication")
	}

	// Drain any additional publications.
	time.Sleep(200 * time.Millisecond)
	for {
		select {
		case ob := <-obCh:
			lastOB = ob
		default:
			goto done
		}
	}
done:

	// The bid at 30000 should have been updated to size 2.0.
	bids := lastOB.Bids()
	if len(bids) == 0 {
		t.Fatal("no bids in order book")
	}
	if bids[0].Price == nil || *bids[0].Price != 30000.00 {
		t.Errorf("expected best bid price=30000, got %v", bids[0].Price)
	}
	if bids[0].Size == nil || *bids[0].Size != 2.0 {
		t.Errorf("expected best bid size=2.0, got %v", bids[0].Size)
	}

	// The ask at 30001 should have been updated to size 0.8.
	asks := lastOB.Asks()
	if len(asks) == 0 {
		t.Fatal("no asks in order book")
	}
	if asks[0].Price == nil || *asks[0].Price != 30001.00 {
		t.Errorf("expected best ask price=30001, got %v", asks[0].Price)
	}
	if asks[0].Size == nil || *asks[0].Size != 0.8 {
		t.Errorf("expected best ask size=0.8, got %v", asks[0].Size)
	}
}

func TestBinance_Trade(t *testing.T) {
	snapshot := snapshotResponse{
		LastUpdateID: 100,
		Bids:         [][]string{{"30000.00", "1.0"}},
		Asks:         [][]string{{"30001.00", "1.0"}},
	}

	tradeEvent := tradeMsg{
		EventType:    "trade",
		EventTime:    time.Now().UnixMilli(),
		Symbol:       "BTCUSDT",
		Price:        "30000.50",
		Quantity:     "0.5",
		TradeTime:    time.Now().UnixMilli(),
		IsBuyerMaker: true, // buyer is maker => sell
	}

	tradeData, _ := json.Marshal(tradeEvent)
	combined := combinedMsg{
		Stream: "btcusdt@trade",
		Data:   tradeData,
	}
	combinedData, _ := json.Marshal(combined)

	wsSrv, wsURL := mockWSServer(t, func(conn *websocket.Conn) {
		_ = conn.WriteMessage(websocket.TextMessage, combinedData)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer wsSrv.Close()

	c, restSrv := newTestConnector(t, wsURL, snapshot, []string{"BTCUSDT"})
	defer restSrv.Close()

	_, tradeCh := c.bus.Trades.Subscribe(16)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer func() { _ = c.StopAsync(context.Background()) }()

	var trade models.Trade
	ok := waitForCondition(3*time.Second, func() bool {
		select {
		case trade = <-tradeCh:
			return true
		default:
			return false
		}
	})
	if !ok {
		t.Fatal("timed out waiting for trade")
	}

	if trade.Symbol != "BTCUSDT" {
		t.Errorf("expected symbol BTCUSDT, got %s", trade.Symbol)
	}
	if !trade.Price.Equal(decimal.RequireFromString("30000.50")) {
		t.Errorf("expected price 30000.50, got %s", trade.Price.String())
	}
	if !trade.Size.Equal(decimal.RequireFromString("0.5")) {
		t.Errorf("expected size 0.5, got %s", trade.Size.String())
	}
	// m=true => isBuy should be false (buyer is maker, taker is seller)
	if trade.IsBuy == nil || *trade.IsBuy != false {
		t.Errorf("expected IsBuy=false (buyer is maker), got %v", trade.IsBuy)
	}
}

func TestBinance_SnapshotAndDelta(t *testing.T) {
	snapshot := snapshotResponse{
		LastUpdateID: 200,
		Bids:         [][]string{{"50000.00", "1.0"}, {"49999.00", "2.0"}},
		Asks:         [][]string{{"50001.00", "1.0"}, {"50002.00", "2.0"}},
	}

	// Delta that arrives after the snapshot: adds a new bid level and updates
	// an existing ask level.
	delta := depthMsg{
		EventType:     "depthUpdate",
		EventTime:     time.Now().UnixMilli(),
		Symbol:        "BTCUSDT",
		FirstUpdateID: 201,
		FinalUpdateID: 203,
		Bids:          [][]string{{"50000.50", "3.0"}}, // new level between existing
		Asks:          [][]string{{"50001.00", "1.5"}}, // update existing
	}
	deltaData, _ := json.Marshal(delta)
	combined := combinedMsg{
		Stream: "btcusdt@depth@100ms",
		Data:   deltaData,
	}
	combinedData, _ := json.Marshal(combined)

	wsSrv, wsURL := mockWSServer(t, func(conn *websocket.Conn) {
		_ = conn.WriteMessage(websocket.TextMessage, combinedData)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer wsSrv.Close()

	c, restSrv := newTestConnector(t, wsURL, snapshot, []string{"BTCUSDT"})
	defer restSrv.Close()

	_, obCh := c.bus.OrderBooks.Subscribe(16)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer func() { _ = c.StopAsync(context.Background()) }()

	// Collect order book publications until we see the delta applied.
	var lastOB *models.OrderBook
	ok := waitForCondition(3*time.Second, func() bool {
		select {
		case ob := <-obCh:
			lastOB = ob
			// Check if delta was applied: look for bid at 50000.50.
			bids := ob.Bids()
			for _, b := range bids {
				if b.Price != nil && *b.Price == 50000.50 {
					return true
				}
			}
			return false
		default:
			return false
		}
	})
	if !ok {
		t.Fatal("timed out waiting for delta to be applied to order book")
	}

	// Verify the order book state.
	bids := lastOB.Bids()
	if len(bids) < 3 {
		t.Fatalf("expected at least 3 bid levels, got %d", len(bids))
	}

	// Bids sorted descending: 50000.50 (new), 50000.00, 49999.00
	if *bids[0].Price != 50000.50 {
		t.Errorf("expected best bid=50000.50, got %v", *bids[0].Price)
	}
	if *bids[0].Size != 3.0 {
		t.Errorf("expected best bid size=3.0, got %v", *bids[0].Size)
	}

	// Ask at 50001.00 should be updated to 1.5.
	asks := lastOB.Asks()
	if len(asks) == 0 {
		t.Fatal("no asks")
	}
	if *asks[0].Price != 50001.00 {
		t.Errorf("expected best ask=50001.00, got %v", *asks[0].Price)
	}
	if *asks[0].Size != 1.5 {
		t.Errorf("expected best ask size=1.5, got %v", *asks[0].Size)
	}
}

func TestBinance_DeleteLevel(t *testing.T) {
	snapshot := snapshotResponse{
		LastUpdateID: 300,
		Bids:         [][]string{{"40000.00", "1.0"}, {"39999.00", "2.0"}},
		Asks:         [][]string{{"40001.00", "1.0"}, {"40002.00", "0.5"}},
	}

	// Delta that deletes bid at 39999.00 (qty=0) and ask at 40002.00 (qty=0).
	delta := depthMsg{
		EventType:     "depthUpdate",
		EventTime:     time.Now().UnixMilli(),
		Symbol:        "BTCUSDT",
		FirstUpdateID: 301,
		FinalUpdateID: 302,
		Bids:          [][]string{{"39999.00", "0"}},
		Asks:          [][]string{{"40002.00", "0"}},
	}
	deltaData, _ := json.Marshal(delta)
	combined := combinedMsg{
		Stream: "btcusdt@depth@100ms",
		Data:   deltaData,
	}
	combinedData, _ := json.Marshal(combined)

	wsSrv, wsURL := mockWSServer(t, func(conn *websocket.Conn) {
		_ = conn.WriteMessage(websocket.TextMessage, combinedData)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer wsSrv.Close()

	c, restSrv := newTestConnector(t, wsURL, snapshot, []string{"BTCUSDT"})
	defer restSrv.Close()

	_, obCh := c.bus.OrderBooks.Subscribe(16)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer func() { _ = c.StopAsync(context.Background()) }()

	// Wait for order book publications and check for deletions.
	var lastOB *models.OrderBook
	ok := waitForCondition(3*time.Second, func() bool {
		select {
		case ob := <-obCh:
			lastOB = ob
			return true
		default:
			return false
		}
	})
	if !ok {
		t.Fatal("timed out waiting for order book publication")
	}

	// Drain additional publications.
	time.Sleep(200 * time.Millisecond)
	for {
		select {
		case ob := <-obCh:
			lastOB = ob
		default:
			goto done
		}
	}
done:

	// After snapshot (2 bids, 2 asks) + delta (delete 1 bid, delete 1 ask)
	// we should have 1 bid and 1 ask.
	bids := lastOB.Bids()
	if len(bids) != 1 {
		t.Errorf("expected 1 bid after delete, got %d", len(bids))
		for _, b := range bids {
			t.Logf("  bid: price=%v size=%v", b.Price, b.Size)
		}
	} else {
		if *bids[0].Price != 40000.00 {
			t.Errorf("expected remaining bid price=40000, got %v", *bids[0].Price)
		}
	}

	asks := lastOB.Asks()
	if len(asks) != 1 {
		t.Errorf("expected 1 ask after delete, got %d", len(asks))
		for _, a := range asks {
			t.Logf("  ask: price=%v size=%v", a.Price, a.Size)
		}
	} else {
		if *asks[0].Price != 40001.00 {
			t.Errorf("expected remaining ask price=40001, got %v", *asks[0].Price)
		}
	}
}

func TestBinance_StaleDeltas_Dropped(t *testing.T) {
	// Verify that deltas with FinalUpdateID < lastUpdateID+1 are ignored.
	snapshot := snapshotResponse{
		LastUpdateID: 500,
		Bids:         [][]string{{"10000.00", "1.0"}},
		Asks:         [][]string{{"10001.00", "1.0"}},
	}

	// Stale delta: FinalUpdateID=499 < 500+1
	staleDelta := depthMsg{
		EventType:     "depthUpdate",
		EventTime:     time.Now().UnixMilli(),
		Symbol:        "BTCUSDT",
		FirstUpdateID: 498,
		FinalUpdateID: 499,
		Bids:          [][]string{{"10000.00", "9999.0"}}, // should NOT be applied
		Asks:          [][]string{},
	}

	// Valid delta: FinalUpdateID=502 >= 500+1
	validDelta := depthMsg{
		EventType:     "depthUpdate",
		EventTime:     time.Now().UnixMilli(),
		Symbol:        "BTCUSDT",
		FirstUpdateID: 501,
		FinalUpdateID: 502,
		Bids:          [][]string{{"10000.00", "5.0"}},
		Asks:          [][]string{},
	}

	staleData, _ := json.Marshal(staleDelta)
	staleCombined, _ := json.Marshal(combinedMsg{
		Stream: "btcusdt@depth@100ms",
		Data:   staleData,
	})

	validData, _ := json.Marshal(validDelta)
	validCombined, _ := json.Marshal(combinedMsg{
		Stream: "btcusdt@depth@100ms",
		Data:   validData,
	})

	wsSrv, wsURL := mockWSServer(t, func(conn *websocket.Conn) {
		_ = conn.WriteMessage(websocket.TextMessage, staleCombined)
		time.Sleep(50 * time.Millisecond)
		_ = conn.WriteMessage(websocket.TextMessage, validCombined)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer wsSrv.Close()

	c, restSrv := newTestConnector(t, wsURL, snapshot, []string{"BTCUSDT"})
	defer restSrv.Close()

	_, obCh := c.bus.OrderBooks.Subscribe(16)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer func() { _ = c.StopAsync(context.Background()) }()

	// Wait for order book with valid delta applied (bid size=5.0).
	var lastOB *models.OrderBook
	ok := waitForCondition(3*time.Second, func() bool {
		select {
		case ob := <-obCh:
			lastOB = ob
			bids := ob.Bids()
			if len(bids) > 0 && bids[0].Size != nil && *bids[0].Size == 5.0 {
				return true
			}
			return false
		default:
			return false
		}
	})
	if !ok {
		// Check latest state.
		if lastOB != nil {
			bids := lastOB.Bids()
			if len(bids) > 0 {
				t.Fatalf("timed out: last bid size=%v (expected 5.0)", *bids[0].Size)
			}
		}
		t.Fatal("timed out waiting for valid delta to be applied")
	}

	bids := lastOB.Bids()
	// The stale delta (size=9999) should NOT have been applied.
	if *bids[0].Size == 9999.0 {
		t.Error("stale delta was incorrectly applied")
	}
	if *bids[0].Size != 5.0 {
		t.Errorf("expected bid size=5.0, got %v", *bids[0].Size)
	}
}

func TestBinance_BuildStreams(t *testing.T) {
	bus := newTestBus()
	c := New(bus, nil, slog.Default())
	c.settings.UpdateIntervalMs = 100

	streams := c.buildStreams([]string{"BTCUSDT", "ETHUSDT"})

	expected := []string{
		"btcusdt@depth@100ms",
		"btcusdt@trade",
		"ethusdt@depth@100ms",
		"ethusdt@trade",
	}

	if len(streams) != len(expected) {
		t.Fatalf("expected %d streams, got %d", len(expected), len(streams))
	}

	for i, s := range streams {
		if s != expected[i] {
			t.Errorf("stream[%d]: expected %q, got %q", i, expected[i], s)
		}
	}
}

func TestBinance_WSBaseURL(t *testing.T) {
	bus := newTestBus()

	// Global
	c := New(bus, nil, slog.Default())
	c.settings.IsNonUS = true
	if got := c.wsBaseURL(); got != wsGlobalBase {
		t.Errorf("expected %s, got %s", wsGlobalBase, got)
	}

	// US
	c2 := New(bus, nil, slog.Default())
	c2.settings.IsNonUS = false
	if got := c2.wsBaseURL(); got != wsUSBase {
		t.Errorf("expected %s, got %s", wsUSBase, got)
	}
}

func TestBinance_DirectMessage(t *testing.T) {
	// Test that non-combined stream messages (direct trade) are handled.
	snapshot := snapshotResponse{
		LastUpdateID: 100,
		Bids:         [][]string{{"30000.00", "1.0"}},
		Asks:         [][]string{{"30001.00", "1.0"}},
	}

	directTrade := tradeMsg{
		EventType:    "trade",
		EventTime:    time.Now().UnixMilli(),
		Symbol:       "BTCUSDT",
		Price:        "29999.99",
		Quantity:     "1.0",
		TradeTime:    time.Now().UnixMilli(),
		IsBuyerMaker: false,
	}
	directData, _ := json.Marshal(directTrade)

	wsSrv, wsURL := mockWSServer(t, func(conn *websocket.Conn) {
		// Small delay to ensure readLoop is scheduled.
		time.Sleep(50 * time.Millisecond)
		_ = conn.WriteMessage(websocket.TextMessage, directData)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer wsSrv.Close()

	c, restSrv := newTestConnector(t, wsURL, snapshot, []string{"BTCUSDT"})
	defer restSrv.Close()

	_, tradeCh := c.bus.Trades.Subscribe(16)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer func() { _ = c.StopAsync(context.Background()) }()

	var trade models.Trade
	ok := waitForCondition(3*time.Second, func() bool {
		select {
		case trade = <-tradeCh:
			return true
		default:
			return false
		}
	})
	if !ok {
		t.Fatal("timed out waiting for direct trade")
	}

	if trade.Price.String() != "29999.99" {
		t.Errorf("expected price=29999.99, got %s", trade.Price.String())
	}
	// m=false => buyer is taker => isBuy=true
	if trade.IsBuy == nil || *trade.IsBuy != true {
		t.Errorf("expected IsBuy=true, got %v", trade.IsBuy)
	}
}

func TestBinance_BufferedDeltasAppliedAfterSnapshot(t *testing.T) {
	// Verifies that deltas arriving before the REST snapshot are buffered and
	// applied once the snapshot arrives.
	snapshot := snapshotResponse{
		LastUpdateID: 100,
		Bids:         [][]string{{"20000.00", "1.0"}},
		Asks:         [][]string{{"20001.00", "1.0"}},
	}

	// Delta that would arrive before snapshot completes.
	earlyDelta := depthMsg{
		EventType:     "depthUpdate",
		EventTime:     time.Now().UnixMilli(),
		Symbol:        "BTCUSDT",
		FirstUpdateID: 101,
		FinalUpdateID: 102,
		Bids:          [][]string{{"20000.00", "7.0"}},
		Asks:          [][]string{},
	}
	earlyData, _ := json.Marshal(earlyDelta)
	earlyCombined, _ := json.Marshal(combinedMsg{
		Stream: "btcusdt@depth@100ms",
		Data:   earlyData,
	})

	wsSrv, wsURL := mockWSServer(t, func(conn *websocket.Conn) {
		// Send delta immediately (before snapshot can complete).
		_ = conn.WriteMessage(websocket.TextMessage, earlyCombined)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer wsSrv.Close()

	// Use a slow REST server to ensure the delta arrives before snapshot.
	restData, _ := json.Marshal(snapshot)
	restSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond) // slow response
		w.Header().Set("Content-Type", "application/json")
		w.Write(restData)
	}))
	defer restSrv.Close()

	bus := newTestBus()
	c := New(bus, nil, slog.Default())
	c.settings.Symbols = []string{"BTCUSDT"}
	c.wsURL = wsURL
	c.restURL = restSrv.URL
	c.httpClient = restSrv.Client()

	_, obCh := c.bus.OrderBooks.Subscribe(16)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer func() { _ = c.StopAsync(context.Background()) }()

	// Wait for order book with delta applied (bid size=7.0).
	ok := waitForCondition(3*time.Second, func() bool {
		select {
		case ob := <-obCh:
			bids := ob.Bids()
			if len(bids) > 0 && bids[0].Size != nil && *bids[0].Size == 7.0 {
				return true
			}
			return false
		default:
			return false
		}
	})
	if !ok {
		t.Fatal("timed out: buffered delta was not applied after snapshot")
	}
}

func TestBinance_MultipleSymbols(t *testing.T) {
	snapshot := snapshotResponse{
		LastUpdateID: 100,
		Bids:         [][]string{{"1000.00", "1.0"}},
		Asks:         [][]string{{"1001.00", "1.0"}},
	}

	// Two trade events for different symbols.
	trade1 := tradeMsg{
		EventType: "trade", EventTime: time.Now().UnixMilli(),
		Symbol: "BTCUSDT", Price: "50000.00", Quantity: "1.0",
		TradeTime: time.Now().UnixMilli(), IsBuyerMaker: false,
	}
	trade2 := tradeMsg{
		EventType: "trade", EventTime: time.Now().UnixMilli(),
		Symbol: "ETHUSDT", Price: "3000.00", Quantity: "2.0",
		TradeTime: time.Now().UnixMilli(), IsBuyerMaker: true,
	}

	data1, _ := json.Marshal(trade1)
	combined1, _ := json.Marshal(combinedMsg{Stream: "btcusdt@trade", Data: data1})
	data2, _ := json.Marshal(trade2)
	combined2, _ := json.Marshal(combinedMsg{Stream: "ethusdt@trade", Data: data2})

	wsSrv, wsURL := mockWSServer(t, func(conn *websocket.Conn) {
		_ = conn.WriteMessage(websocket.TextMessage, combined1)
		_ = conn.WriteMessage(websocket.TextMessage, combined2)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer wsSrv.Close()

	c, restSrv := newTestConnector(t, wsURL, snapshot, []string{"BTCUSDT", "ETHUSDT"})
	defer restSrv.Close()

	_, tradeCh := c.bus.Trades.Subscribe(16)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer func() { _ = c.StopAsync(context.Background()) }()

	// Collect 2 trades.
	symbols := make(map[string]bool)
	ok := waitForCondition(3*time.Second, func() bool {
		select {
		case trade := <-tradeCh:
			symbols[trade.Symbol] = true
			return len(symbols) >= 2
		default:
			return false
		}
	})
	if !ok {
		t.Fatalf("timed out: received trades for symbols %v, expected 2", symbols)
	}

	if !symbols["BTCUSDT"] {
		t.Error("missing trade for BTCUSDT")
	}
	if !symbols["ETHUSDT"] {
		t.Error("missing trade for ETHUSDT")
	}
}

func TestBinance_ParseLevels(t *testing.T) {
	entries := [][]string{
		{"100.50", "2.0"},
		{"99.00", "3.5"},
		{"bad", "1.0"},  // invalid price
		{"100.00"},      // missing qty
	}

	bids := parseLevels(entries, true)
	if len(bids) != 2 {
		t.Fatalf("expected 2 valid levels, got %d", len(bids))
	}

	if !bids[0].IsBid {
		t.Error("expected IsBid=true")
	}
	if *bids[0].Price != 100.50 {
		t.Errorf("expected price=100.50, got %v", *bids[0].Price)
	}
	if *bids[0].Size != 2.0 {
		t.Errorf("expected size=2.0, got %v", *bids[0].Size)
	}
	if *bids[1].Price != 99.00 {
		t.Errorf("expected price=99.0, got %v", *bids[1].Price)
	}

	asks := parseLevels(entries[:2], false)
	if asks[0].IsBid {
		t.Error("expected IsBid=false for asks")
	}
}

func TestBinance_NoSymbols_Error(t *testing.T) {
	bus := newTestBus()
	c := New(bus, nil, slog.Default())
	c.settings.Symbols = []string{}

	err := c.StartAsync(context.Background())
	if err == nil {
		t.Fatal("expected error with no symbols configured")
	}
	if !strings.Contains(err.Error(), "no symbols") {
		t.Errorf("unexpected error: %v", err)
	}
	_ = c.StopAsync(context.Background())
}

func TestBinance_DispatchDirect_Unit(t *testing.T) {
	// Unit test verifying that dispatchDirect correctly parses a direct
	// (non-combined stream) trade message despite Go's case-insensitive
	// JSON key matching ("e" vs "E").
	bus := newTestBus()
	c := New(bus, nil, slog.Default())
	c.settings.Symbols = []string{"BTCUSDT"}
	c.ParseSymbols("BTCUSDT")

	// Set up the symbol book so handleTrade can look up the mid price.
	ob := models.NewOrderBook("BTCUSDT", 8, 10)
	c.books.Store("btcusdt", &symbolBook{
		ob:           ob,
		snapshotDone: true,
	})

	_, tradeCh := bus.Trades.Subscribe(16)

	directTrade := tradeMsg{
		EventType:    "trade",
		EventTime:    time.Now().UnixMilli(),
		Symbol:       "BTCUSDT",
		Price:        "12345.67",
		Quantity:     "0.1",
		TradeTime:    time.Now().UnixMilli(),
		IsBuyerMaker: false,
	}
	raw, _ := json.Marshal(directTrade)

	c.dispatchDirect(raw)

	select {
	case trade := <-tradeCh:
		if trade.Price.String() != "12345.67" {
			t.Errorf("expected price=12345.67, got %s", trade.Price.String())
		}
	case <-time.After(1 * time.Second):
		t.Fatal("trade was not published via dispatchDirect")
	}
}
