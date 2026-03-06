package recorder

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

func TestRecorderOrderBook(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	bus := eventbus.NewBus(logger)
	defer bus.Close()

	rec, err := New(bus, dir, logger)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	rec.Start(ctx)

	// Build an order book with some levels
	ob := models.NewOrderBook("BTCUSD", 2, 10)
	ob.ProviderName = "test-exchange"
	ob.Sequence = 42

	bidPrice := 50000.0
	bidSize := 1.5
	askPrice := 50001.0
	askSize := 2.0

	bids := []models.BookItem{
		{Price: &bidPrice, Size: &bidSize, IsBid: true},
	}
	asks := []models.BookItem{
		{Price: &askPrice, Size: &askSize, IsBid: false},
	}
	ob.LoadData(asks, bids)

	// Publish to bus
	bus.OrderBooks.Publish(ob)

	// Wait for the goroutine to process
	time.Sleep(200 * time.Millisecond)

	cancel()
	rec.Stop()

	// Verify the orderbook file was written
	obDir := filepath.Join(dir, "orderbooks")
	entries, err := os.ReadDir(obDir)
	if err != nil {
		t.Fatalf("read orderbooks dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one file in orderbooks/")
	}

	// Read and decode the first file
	path := filepath.Join(obDir, entries[0].Name())
	records := readGzipJSONL[OrderBookRecord](t, path)

	if len(records) == 0 {
		t.Fatal("expected at least one OrderBookRecord")
	}

	got := records[0]
	if got.Type != "orderbook" {
		t.Fatalf("type: expected %q, got %q", "orderbook", got.Type)
	}
	if got.Symbol != "BTCUSD" {
		t.Fatalf("symbol: expected %q, got %q", "BTCUSD", got.Symbol)
	}
	if got.Provider != "test-exchange" {
		t.Fatalf("provider: expected %q, got %q", "test-exchange", got.Provider)
	}
	if got.Sequence != 42 {
		t.Fatalf("sequence: expected 42, got %d", got.Sequence)
	}
	if len(got.Bids) != 1 {
		t.Fatalf("bids: expected 1, got %d", len(got.Bids))
	}
	if got.Bids[0].Price != bidPrice {
		t.Fatalf("bid price: expected %f, got %f", bidPrice, got.Bids[0].Price)
	}
	if got.Bids[0].Size != bidSize {
		t.Fatalf("bid size: expected %f, got %f", bidSize, got.Bids[0].Size)
	}
	if len(got.Asks) != 1 {
		t.Fatalf("asks: expected 1, got %d", len(got.Asks))
	}
	if got.Asks[0].Price != askPrice {
		t.Fatalf("ask price: expected %f, got %f", askPrice, got.Asks[0].Price)
	}
	if got.ExchangeTS == 0 {
		t.Fatal("exchange_ts should be non-zero")
	}
	if got.LocalTS == 0 {
		t.Fatal("local_ts should be non-zero")
	}
	if got.MidPrice == 0 {
		t.Fatal("mid price should be non-zero")
	}
	if got.Spread == 0 {
		t.Fatal("spread should be non-zero")
	}
}

func TestRecorderTrade(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	bus := eventbus.NewBus(logger)
	defer bus.Close()

	rec, err := New(bus, dir, logger)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	rec.Start(ctx)

	isBuy := true
	trade := models.Trade{
		Symbol:         "ETHUSD",
		ProviderName:   "test-exchange",
		Price:          decimal.NewFromFloat(3500.25),
		Size:           decimal.NewFromFloat(10.0),
		Timestamp:      time.Now(),
		IsBuy:          &isBuy,
		MarketMidPrice: 3500.00,
	}

	bus.Trades.Publish(trade)

	time.Sleep(200 * time.Millisecond)

	cancel()
	rec.Stop()

	// Verify the trade file was written
	tradeDir := filepath.Join(dir, "trades")
	entries, err := os.ReadDir(tradeDir)
	if err != nil {
		t.Fatalf("read trades dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one file in trades/")
	}

	path := filepath.Join(tradeDir, entries[0].Name())
	records := readGzipJSONL[TradeRecord](t, path)

	if len(records) == 0 {
		t.Fatal("expected at least one TradeRecord")
	}

	got := records[0]
	if got.Type != "trade" {
		t.Fatalf("type: expected %q, got %q", "trade", got.Type)
	}
	if got.Symbol != "ETHUSD" {
		t.Fatalf("symbol: expected %q, got %q", "ETHUSD", got.Symbol)
	}
	if got.Provider != "test-exchange" {
		t.Fatalf("provider: expected %q, got %q", "test-exchange", got.Provider)
	}
	if got.Price != "3500.25" {
		t.Fatalf("price: expected %q, got %q", "3500.25", got.Price)
	}
	if got.Size != "10" {
		t.Fatalf("size: expected %q, got %q", "10", got.Size)
	}
	if got.IsBuy == nil || *got.IsBuy != true {
		t.Fatal("is_buy: expected true")
	}
	if got.MidPrice != 3500.00 {
		t.Fatalf("mid: expected 3500.00, got %f", got.MidPrice)
	}
	if got.ExchangeTS == 0 {
		t.Fatal("exchange_ts should be non-zero")
	}
	if got.LocalTS == 0 {
		t.Fatal("local_ts should be non-zero")
	}
}

func TestRecorderNilPriceLevelsSkipped(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	bus := eventbus.NewBus(logger)
	defer bus.Close()

	rec, err := New(bus, dir, logger)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	rec.Start(ctx)

	ob := models.NewOrderBook("BTCUSD", 2, 10)
	ob.ProviderName = "test"

	bidPrice := 50000.0
	bidSize := 1.0
	askPrice := 50001.0
	askSize := 2.0

	bids := []models.BookItem{
		{Price: &bidPrice, Size: &bidSize, IsBid: true},
		{Price: nil, Size: &bidSize, IsBid: true},  // nil price, should be skipped
		{Price: &bidPrice, Size: nil, IsBid: true},  // nil size, should be skipped
	}
	asks := []models.BookItem{
		{Price: &askPrice, Size: &askSize, IsBid: false},
	}
	ob.LoadData(asks, bids)

	bus.OrderBooks.Publish(ob)
	time.Sleep(200 * time.Millisecond)

	cancel()
	rec.Stop()

	obDir := filepath.Join(dir, "orderbooks")
	entries, err := os.ReadDir(obDir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one file")
	}

	path := filepath.Join(obDir, entries[0].Name())
	records := readGzipJSONL[OrderBookRecord](t, path)

	if len(records) == 0 {
		t.Fatal("expected at least one record")
	}

	// LoadData filters out nil-Price items during sort, but our recorder
	// also filters them. Only the valid bid should remain.
	got := records[0]
	if len(got.Bids) != 1 {
		t.Fatalf("expected 1 valid bid, got %d", len(got.Bids))
	}
	if len(got.Asks) != 1 {
		t.Fatalf("expected 1 ask, got %d", len(got.Asks))
	}
}

func TestRecorderStopWithoutEvents(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	bus := eventbus.NewBus(logger)
	defer bus.Close()

	rec, err := New(bus, dir, logger)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	rec.Start(ctx)

	cancel()
	rec.Stop()

	// Should not panic or hang; directories should exist but be empty or
	// have no data files.
	for _, sub := range []string{"orderbooks", "trades"} {
		info, err := os.Stat(filepath.Join(dir, sub))
		if err != nil {
			t.Fatalf("stat %s: %v", sub, err)
		}
		if !info.IsDir() {
			t.Fatalf("expected %s to be a directory", sub)
		}
	}
}

// readGzipJSONL is a test helper that reads a gzip-compressed JSONL file
// and decodes each line into a value of type T.
func readGzipJSONL[T any](t *testing.T, path string) []T {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gz.Close()

	var results []T
	scanner := bufio.NewScanner(gz)
	for scanner.Scan() {
		var v T
		if err := json.Unmarshal(scanner.Bytes(), &v); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		results = append(results, v)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner: %v", err)
	}
	return results
}
