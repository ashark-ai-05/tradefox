package kiyotaka

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestFetchOI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-kiyotaka-key") != "test-key" {
			t.Error("missing API key header")
		}
		if r.URL.Path != "/v1/open-interest" {
			t.Errorf("path = %s, want /v1/open-interest", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"openInterest": 50000000.0, "timestamp": 1709150400000.0},
			},
		})
	}))
	defer srv.Close()

	c := NewClient(Config{APIKey: "test-key", BaseURL: srv.URL}, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	dp, err := c.FetchOI(context.Background(), "binance", "BTC-USDT", "PERPETUAL")
	if err != nil {
		t.Fatalf("FetchOI: %v", err)
	}
	if dp == nil {
		t.Fatal("FetchOI returned nil")
	}
	if dp.Value != 50000000.0 {
		t.Errorf("OI value = %f, want 50000000", dp.Value)
	}
}

func TestFetchFundingRate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-kiyotaka-key") != "test-key" {
			t.Error("missing API key header")
		}
		if r.URL.Path != "/v1/funding-rate" {
			t.Errorf("path = %s, want /v1/funding-rate", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"fundingRate": 0.0005, "timestamp": 1709150400000.0},
			},
		})
	}))
	defer srv.Close()

	c := NewClient(Config{APIKey: "test-key", BaseURL: srv.URL}, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	dp, err := c.FetchFundingRate(context.Background(), "binance", "BTC-USDT", "PERPETUAL")
	if err != nil {
		t.Fatalf("FetchFundingRate: %v", err)
	}
	if dp.Rate != 0.0005 {
		t.Errorf("rate = %f, want 0.0005", dp.Rate)
	}
}

func TestFetchLiquidations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-kiyotaka-key") != "test-key" {
			t.Error("missing API key header")
		}
		if r.URL.Path != "/v1/liquidations" {
			t.Errorf("path = %s, want /v1/liquidations", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"buyVolume": 1200000.0, "sellVolume": 850000.0, "timestamp": 1709150400000.0},
			},
		})
	}))
	defer srv.Close()

	c := NewClient(Config{APIKey: "test-key", BaseURL: srv.URL}, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	dp, err := c.FetchLiquidations(context.Background(), "binance", "BTC-USDT", "PERPETUAL")
	if err != nil {
		t.Fatalf("FetchLiquidations: %v", err)
	}
	if dp == nil {
		t.Fatal("FetchLiquidations returned nil")
	}
	if dp.BuyVolume != 1200000.0 {
		t.Errorf("BuyVolume = %f, want 1200000", dp.BuyVolume)
	}
	if dp.SellVolume != 850000.0 {
		t.Errorf("SellVolume = %f, want 850000", dp.SellVolume)
	}
}

func TestFetchOHLCV(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-kiyotaka-key") != "test-key" {
			t.Error("missing API key header")
		}
		if r.URL.Path != "/v1/ohlcv" {
			t.Errorf("path = %s, want /v1/ohlcv", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"open": 42000.0, "high": 43500.0, "low": 41800.0, "close": 43100.0, "volume": 500.0, "timestamp": 1709150400000.0},
			},
		})
	}))
	defer srv.Close()

	c := NewClient(Config{APIKey: "test-key", BaseURL: srv.URL}, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	dp, err := c.FetchOHLCV(context.Background(), "binance", "BTC-USDT", "PERPETUAL", "1h")
	if err != nil {
		t.Fatalf("FetchOHLCV: %v", err)
	}
	if dp == nil {
		t.Fatal("FetchOHLCV returned nil")
	}
	if dp.Open != 42000.0 {
		t.Errorf("Open = %f, want 42000", dp.Open)
	}
	if dp.Close != 43100.0 {
		t.Errorf("Close = %f, want 43100", dp.Close)
	}
}
