package backfill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckpoint_SaveLoad(t *testing.T) {
	store, err := NewCheckpointStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	cp := Checkpoint{Symbol: "SOLUSDT", DataType: "trades", LastTS: 1700000000000}
	if err := store.Save(cp); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load("SOLUSDT", "trades")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LastTS != 1700000000000 {
		t.Errorf("expected lastTS 1700000000000, got %d", loaded.LastTS)
	}
	if loaded.Symbol != "SOLUSDT" {
		t.Errorf("expected symbol SOLUSDT, got %s", loaded.Symbol)
	}
	if loaded.DataType != "trades" {
		t.Errorf("expected dataType trades, got %s", loaded.DataType)
	}
	if loaded.UpdatedAt.IsZero() {
		t.Error("expected non-zero UpdatedAt")
	}
}

func TestCheckpoint_LoadMissing(t *testing.T) {
	store, err := NewCheckpointStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	cp, err := store.Load("SOLUSDT", "trades")
	if err != nil {
		t.Fatal(err)
	}
	if cp.LastTS != 0 {
		t.Errorf("expected zero checkpoint, got lastTS %d", cp.LastTS)
	}
}

func TestCheckpoint_Overwrite(t *testing.T) {
	store, err := NewCheckpointStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	cp1 := Checkpoint{Symbol: "SOLUSDT", DataType: "trades", LastTS: 1700000000000}
	if err := store.Save(cp1); err != nil {
		t.Fatal(err)
	}

	cp2 := Checkpoint{Symbol: "SOLUSDT", DataType: "trades", LastTS: 1700000060000}
	if err := store.Save(cp2); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load("SOLUSDT", "trades")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LastTS != 1700000060000 {
		t.Errorf("expected lastTS 1700000060000 after overwrite, got %d", loaded.LastTS)
	}
}

func TestCheckpoint_MultipleTypes(t *testing.T) {
	store, err := NewCheckpointStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	cpTrades := Checkpoint{Symbol: "SOLUSDT", DataType: "trades", LastTS: 1000}
	cpKlines := Checkpoint{Symbol: "SOLUSDT", DataType: "klines", LastTS: 2000}

	if err := store.Save(cpTrades); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(cpKlines); err != nil {
		t.Fatal(err)
	}

	loadedTrades, err := store.Load("SOLUSDT", "trades")
	if err != nil {
		t.Fatal(err)
	}
	loadedKlines, err := store.Load("SOLUSDT", "klines")
	if err != nil {
		t.Fatal(err)
	}

	if loadedTrades.LastTS != 1000 {
		t.Errorf("expected trades lastTS 1000, got %d", loadedTrades.LastTS)
	}
	if loadedKlines.LastTS != 2000 {
		t.Errorf("expected klines lastTS 2000, got %d", loadedKlines.LastTS)
	}
}

func TestCheckpointStore_CreatesDirectory(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "nested", "path")

	store, err := NewCheckpointStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the .backfill directory was created
	backfillDir := filepath.Join(dir, ".backfill")
	info, err := os.Stat(backfillDir)
	if err != nil {
		t.Fatalf("expected .backfill dir to exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected .backfill to be a directory")
	}

	// Verify we can save/load in the nested directory
	cp := Checkpoint{Symbol: "BTCUSDT", DataType: "funding", LastTS: 9999}
	if err := store.Save(cp); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load("BTCUSDT", "funding")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LastTS != 9999 {
		t.Errorf("expected lastTS 9999, got %d", loaded.LastTS)
	}
}
