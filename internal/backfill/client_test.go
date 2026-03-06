package backfill

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// newTestClient creates a Client pointing at the given test server with a
// high rate limit so tests run without delay.
func newTestClient(srvURL string) *Client {
	c := NewClient(slog.Default())
	c.baseURL = srvURL
	c.limiter = rate.NewLimiter(rate.Inf, 0) // no rate limiting in tests
	return c
}

func TestFetchAggTrades(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fapi/v1/aggTrades" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		q := r.URL.Query()
		if q.Get("symbol") != "SOLUSDT" {
			t.Errorf("expected symbol SOLUSDT, got %s", q.Get("symbol"))
		}
		if q.Get("startTime") != "1700000000000" {
			t.Errorf("expected startTime 1700000000000, got %s", q.Get("startTime"))
		}
		if q.Get("endTime") != "1700000060000" {
			t.Errorf("expected endTime 1700000060000, got %s", q.Get("endTime"))
		}
		if q.Get("limit") != "1000" {
			t.Errorf("expected limit 1000, got %s", q.Get("limit"))
		}
		json.NewEncoder(w).Encode([]RawTrade{
			{ID: 1, Price: "100.50", Qty: "1.5", Time: 1700000000000, IsMaker: false},
			{ID: 2, Price: "100.60", Qty: "2.0", Time: 1700000001000, IsMaker: true},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)

	trades, err := c.FetchAggTrades(context.Background(), "SOLUSDT", 1700000000000, 1700000060000)
	if err != nil {
		t.Fatal(err)
	}
	if len(trades) != 2 {
		t.Fatalf("expected 2 trades, got %d", len(trades))
	}
	if trades[0].Price != "100.50" {
		t.Errorf("expected price 100.50, got %s", trades[0].Price)
	}
	if trades[0].ID != 1 {
		t.Errorf("expected ID 1, got %d", trades[0].ID)
	}
	if trades[1].IsMaker != true {
		t.Errorf("expected second trade IsMaker=true")
	}
}

func TestFetchKlines(t *testing.T) {
	// Binance klines come as JSON arrays:
	// [openTime,"open","high","low","close","vol",closeTime,"quoteVol",numTrades,"takerBuyBase","takerBuyQuote","ignore"]
	mockKlines := `[
		[1700000000000,"100.50","101.00","100.00","100.80","1500.5",1700000059999,"150050.0",320,"800.0","80200.0","0"],
		[1700000060000,"100.80","101.20","100.50","101.00","1200.3",1700000119999,"121000.0",280,"600.0","60600.0","0"]
	]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fapi/v1/klines" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		q := r.URL.Query()
		if q.Get("symbol") != "SOLUSDT" {
			t.Errorf("expected symbol SOLUSDT, got %s", q.Get("symbol"))
		}
		if q.Get("interval") != "1m" {
			t.Errorf("expected interval 1m, got %s", q.Get("interval"))
		}
		if q.Get("limit") != "1500" {
			t.Errorf("expected limit 1500, got %s", q.Get("limit"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(mockKlines))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)

	klines, err := c.FetchKlines(context.Background(), "SOLUSDT", 1700000000000, 1700000120000)
	if err != nil {
		t.Fatal(err)
	}
	if len(klines) != 2 {
		t.Fatalf("expected 2 klines, got %d", len(klines))
	}

	// Verify the raw messages can be parsed as arrays
	var first []json.RawMessage
	if err := json.Unmarshal(klines[0], &first); err != nil {
		t.Fatalf("failed to parse first kline as array: %v", err)
	}
	if len(first) != 12 {
		t.Errorf("expected 12 fields in kline, got %d", len(first))
	}
}

func TestFetchFundingHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fapi/v1/fundingRate" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		q := r.URL.Query()
		if q.Get("symbol") != "SOLUSDT" {
			t.Errorf("expected symbol SOLUSDT, got %s", q.Get("symbol"))
		}
		if q.Get("limit") != "1000" {
			t.Errorf("expected limit 1000, got %s", q.Get("limit"))
		}
		json.NewEncoder(w).Encode([]RawFunding{
			{Symbol: "SOLUSDT", FundingRate: "0.00010000", FundingTime: 1700006400000},
			{Symbol: "SOLUSDT", FundingRate: "-0.00005000", FundingTime: 1700035200000},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)

	funding, err := c.FetchFundingHistory(context.Background(), "SOLUSDT", 1700000000000, 1700100000000)
	if err != nil {
		t.Fatal(err)
	}
	if len(funding) != 2 {
		t.Fatalf("expected 2 funding records, got %d", len(funding))
	}
	if funding[0].FundingRate != "0.00010000" {
		t.Errorf("expected funding rate 0.00010000, got %s", funding[0].FundingRate)
	}
	if funding[0].Symbol != "SOLUSDT" {
		t.Errorf("expected symbol SOLUSDT, got %s", funding[0].Symbol)
	}
	if funding[1].FundingTime != 1700035200000 {
		t.Errorf("expected funding time 1700035200000, got %d", funding[1].FundingTime)
	}
}

func TestFetchOIHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/futures/data/openInterestHist" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		q := r.URL.Query()
		if q.Get("symbol") != "SOLUSDT" {
			t.Errorf("expected symbol SOLUSDT, got %s", q.Get("symbol"))
		}
		if q.Get("period") != "5m" {
			t.Errorf("expected period 5m, got %s", q.Get("period"))
		}
		if q.Get("limit") != "500" {
			t.Errorf("expected limit 500, got %s", q.Get("limit"))
		}
		json.NewEncoder(w).Encode([]RawOI{
			{Symbol: "SOLUSDT", SumOpenInterest: "5000000.00", SumOpenInterestValue: "100000000.00", Timestamp: 1700000000000},
			{Symbol: "SOLUSDT", SumOpenInterest: "5100000.00", SumOpenInterestValue: "102000000.00", Timestamp: 1700000300000},
			{Symbol: "SOLUSDT", SumOpenInterest: "4900000.00", SumOpenInterestValue: "98000000.00", Timestamp: 1700000600000},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)

	oi, err := c.FetchOIHistory(context.Background(), "SOLUSDT", 1700000000000, 1700001000000)
	if err != nil {
		t.Fatal(err)
	}
	if len(oi) != 3 {
		t.Fatalf("expected 3 OI records, got %d", len(oi))
	}
	if oi[0].SumOpenInterest != "5000000.00" {
		t.Errorf("expected OI 5000000.00, got %s", oi[0].SumOpenInterest)
	}
	if oi[2].Timestamp != 1700000600000 {
		t.Errorf("expected timestamp 1700000600000, got %d", oi[2].Timestamp)
	}
}

func TestFetchAggTrades_APIError(t *testing.T) {
	// Test non-retryable error (500)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"code":-1000,"msg":"Internal server error"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)

	_, err := c.FetchAggTrades(context.Background(), "SOLUSDT", 1700000000000, 1700000060000)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestFetchAggTrades_RateLimitRetryWithCancel(t *testing.T) {
	// Test 429 retry logic — cancel context to prevent long backoff waits
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"code":-1015,"msg":"Too many requests"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := c.FetchAggTrades(ctx, "SOLUSDT", 1700000000000, 1700000060000)
	if err == nil {
		t.Fatal("expected error for persistent 429")
	}
	if attempts < 1 {
		t.Error("expected at least 1 retry attempt")
	}
}

func TestFetchAggTrades_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]RawTrade{})
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := c.FetchAggTrades(ctx, "SOLUSDT", 1700000000000, 1700000060000)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
