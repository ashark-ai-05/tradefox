package recorder

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewRotatingWriter(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "data", "nested")

	w, err := NewRotatingWriter(subDir, "test")
	if err != nil {
		t.Fatalf("NewRotatingWriter: %v", err)
	}
	defer w.Close()

	if w.dir != subDir {
		t.Fatalf("expected dir %q, got %q", subDir, w.dir)
	}

	// Directory should have been created
	info, err := os.Stat(subDir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %q to be a directory", subDir)
	}
}

func TestWriteAndRotate(t *testing.T) {
	dir := t.TempDir()

	w, err := NewRotatingWriter(dir, "ob")
	if err != nil {
		t.Fatalf("NewRotatingWriter: %v", err)
	}

	type record struct {
		Symbol string  `json:"symbol"`
		Price  float64 `json:"price"`
		Qty    int     `json:"qty"`
	}

	rec := record{Symbol: "BTCUSD", Price: 42000.50, Qty: 10}
	if err := w.Write(rec); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if w.Count() != 1 {
		t.Fatalf("expected count 1, got %d", w.Count())
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Determine expected filename based on current UTC hour
	now := time.Now().UTC()
	expectedFile := filepath.Join(dir, "ob_"+now.Format("2006-01-02_15")+".jsonl.gz")

	info, err := os.Stat(expectedFile)
	if err != nil {
		t.Fatalf("expected file %q to exist: %v", expectedFile, err)
	}
	if info.Size() == 0 {
		t.Fatal("expected file to have non-zero size")
	}

	// Decompress and decode
	f, err := os.Open(expectedFile)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gz.Close()

	scanner := bufio.NewScanner(gz)
	if !scanner.Scan() {
		t.Fatal("expected at least one line in JSONL")
	}

	var got record
	if err := json.Unmarshal(scanner.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Symbol != rec.Symbol {
		t.Fatalf("symbol: expected %q, got %q", rec.Symbol, got.Symbol)
	}
	if got.Price != rec.Price {
		t.Fatalf("price: expected %f, got %f", rec.Price, got.Price)
	}
	if got.Qty != rec.Qty {
		t.Fatalf("qty: expected %d, got %d", rec.Qty, got.Qty)
	}
}

func TestWriteMultipleRecords(t *testing.T) {
	dir := t.TempDir()

	w, err := NewRotatingWriter(dir, "trades")
	if err != nil {
		t.Fatalf("NewRotatingWriter: %v", err)
	}

	type trade struct {
		ID    int    `json:"id"`
		Side  string `json:"side"`
	}

	records := []trade{
		{ID: 1, Side: "buy"},
		{ID: 2, Side: "sell"},
		{ID: 3, Side: "buy"},
	}

	for _, r := range records {
		if err := w.Write(r); err != nil {
			t.Fatalf("Write(%v): %v", r, err)
		}
	}

	if w.Count() != 3 {
		t.Fatalf("expected count 3, got %d", w.Count())
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Read back and verify all records
	now := time.Now().UTC()
	path := filepath.Join(dir, "trades_"+now.Format("2006-01-02_15")+".jsonl.gz")

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gz.Close()

	scanner := bufio.NewScanner(gz)
	var decoded []trade
	for scanner.Scan() {
		var tr trade
		if err := json.Unmarshal(scanner.Bytes(), &tr); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		decoded = append(decoded, tr)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner: %v", err)
	}

	if len(decoded) != len(records) {
		t.Fatalf("expected %d records, got %d", len(records), len(decoded))
	}

	for i, want := range records {
		if decoded[i] != want {
			t.Fatalf("record %d: expected %+v, got %+v", i, want, decoded[i])
		}
	}
}

func TestRotatingWriter_WriteAt(t *testing.T) {
	dir := t.TempDir()
	w, err := NewRotatingWriter(dir, "test")
	if err != nil {
		t.Fatal(err)
	}

	// Write records at two different hours
	t1 := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	t2 := time.Date(2026, 1, 15, 11, 30, 0, 0, time.UTC)

	if err := w.WriteAt(map[string]string{"msg": "hour10"}, t1); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteAt(map[string]string{"msg": "hour11"}, t2); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	// Verify two separate files were created
	files, _ := filepath.Glob(filepath.Join(dir, "*.jsonl.gz"))
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
}

func TestCloseWithoutWrite(t *testing.T) {
	dir := t.TempDir()

	w, err := NewRotatingWriter(dir, "empty")
	if err != nil {
		t.Fatalf("NewRotatingWriter: %v", err)
	}

	// Closing without writing should not error
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
