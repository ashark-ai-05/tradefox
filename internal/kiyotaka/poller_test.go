package kiyotaka

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPollerWritesToDisk(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"openInterest": 1000.0, "fundingRate": 0.0001, "timestamp": float64(time.Now().UnixMilli())},
			},
		})
	}))
	defer srv.Close()

	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	client := NewClient(Config{APIKey: "test", BaseURL: srv.URL}, logger)

	symbols := []SymbolConfig{{
		Symbol:   "BTC/USDT",
		Exchange: "binance",
		Pair:     "BTC-USDT",
		Category: "PERPETUAL",
	}}

	p, err := NewPoller(client, symbols, dir, logger)
	if err != nil {
		t.Fatalf("NewPoller: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	p.Start(ctx, 500*time.Millisecond)
	time.Sleep(1500 * time.Millisecond)
	cancel()
	p.Stop()

	matches, _ := filepath.Glob(filepath.Join(dir, "kiyotaka", "kiyotaka_*.jsonl.gz"))
	if len(matches) == 0 {
		t.Fatal("no kiyotaka files written")
	}
}
