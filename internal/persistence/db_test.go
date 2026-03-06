package persistence

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB failed: %v", err)
	}
	defer db.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("database file was not created")
	}
}

func TestLogAndGetTrades(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	now := time.Now()
	trade := TradeRecord{
		Symbol:     "BTCUSDT",
		Side:       "LONG",
		EntryPrice: 67000,
		ExitPrice:  68000,
		Quantity:   0.5,
		PnL:       500,
		PnLPct:    1.49,
		RMultiple:  2.5,
		SetupType:  "breakout",
		Notes:      "clean breakout",
		EntryTime:  now.Add(-time.Hour),
		ExitTime:   now,
		Exchange:   "binance",
		Fees:       1.50,
		Paper:      false,
	}

	if err := db.LogTrade(trade); err != nil {
		t.Fatalf("LogTrade failed: %v", err)
	}

	trades, err := db.GetTrades(now.Add(-2*time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("GetTrades failed: %v", err)
	}
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
	if trades[0].Symbol != "BTCUSDT" {
		t.Errorf("expected BTCUSDT, got %s", trades[0].Symbol)
	}
	if trades[0].PnL != 500 {
		t.Errorf("expected PnL 500, got %f", trades[0].PnL)
	}
}

func TestDailySummary(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	now := time.Now()

	trades := []TradeRecord{
		{Symbol: "BTCUSDT", Side: "LONG", EntryPrice: 67000, ExitPrice: 68000, Quantity: 0.5, PnL: 500, PnLPct: 1.49, RMultiple: 2.0, EntryTime: now.Add(-time.Hour), ExitTime: now, Exchange: "binance"},
		{Symbol: "ETHUSDT", Side: "SHORT", EntryPrice: 3500, ExitPrice: 3600, Quantity: 5, PnL: -500, PnLPct: -2.86, RMultiple: -1.0, EntryTime: now.Add(-30 * time.Minute), ExitTime: now, Exchange: "binance"},
	}
	for _, tr := range trades {
		if err := db.LogTrade(tr); err != nil {
			t.Fatalf("LogTrade failed: %v", err)
		}
	}

	summary, err := db.GetDailySummary(now)
	if err != nil {
		t.Fatalf("GetDailySummary failed: %v", err)
	}
	if summary.TotalTrades != 2 {
		t.Errorf("expected 2 total trades, got %d", summary.TotalTrades)
	}
	if summary.WinCount != 1 {
		t.Errorf("expected 1 win, got %d", summary.WinCount)
	}
}

func TestPerformanceStats(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	now := time.Now()
	for i := 0; i < 10; i++ {
		pnl := 100.0
		if i%3 == 0 {
			pnl = -50
		}
		trade := TradeRecord{
			Symbol: "BTCUSDT", Side: "LONG",
			EntryPrice: 67000, ExitPrice: 67000 + pnl,
			Quantity: 1, PnL: pnl, PnLPct: pnl / 67000 * 100,
			RMultiple: pnl / 50,
			EntryTime: now.Add(-time.Hour), ExitTime: now,
			Exchange: "binance",
		}
		if err := db.LogTrade(trade); err != nil {
			t.Fatalf("LogTrade failed: %v", err)
		}
	}

	stats, err := db.GetPerformanceStats(30)
	if err != nil {
		t.Fatalf("GetPerformanceStats failed: %v", err)
	}
	if stats.TotalTrades != 10 {
		t.Errorf("expected 10 trades, got %d", stats.TotalTrades)
	}
	if stats.WinRate == 0 {
		t.Error("expected non-zero win rate")
	}
}

func TestSettings(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	if err := db.SaveSetting("theme", "midnight"); err != nil {
		t.Fatalf("SaveSetting failed: %v", err)
	}

	val, err := db.GetSetting("theme")
	if err != nil {
		t.Fatalf("GetSetting failed: %v", err)
	}
	if val != "midnight" {
		t.Errorf("expected midnight, got %s", val)
	}

	// Update existing
	if err := db.SaveSetting("theme", "matrix"); err != nil {
		t.Fatalf("SaveSetting update failed: %v", err)
	}
	val, err = db.GetSetting("theme")
	if err != nil {
		t.Fatalf("GetSetting failed: %v", err)
	}
	if val != "matrix" {
		t.Errorf("expected matrix, got %s", val)
	}

	// Non-existent key
	val, err = db.GetSetting("nonexistent")
	if err != nil {
		t.Fatalf("GetSetting should not error for missing key: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string, got %s", val)
	}
}

func testDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	db, err := NewDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	return db
}
