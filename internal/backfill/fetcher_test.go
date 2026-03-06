package backfill

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/recorder"
)

func TestTradeFetcher(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		if page == 1 {
			// Return maxTradesPerReq trades (full page)
			trades := make([]RawTrade, maxTradesPerReq)
			for i := range trades {
				trades[i] = RawTrade{
					ID:      int64(i),
					Price:   "100.00",
					Qty:     "1.0",
					Time:    1700000000000 + int64(i)*1000,
					IsMaker: false,
				}
			}
			json.NewEncoder(w).Encode(trades)
		} else {
			// Return 5 trades (partial page = end of data)
			trades := make([]RawTrade, 5)
			for i := range trades {
				trades[i] = RawTrade{
					ID:      int64(1000 + i),
					Price:   "101.00",
					Qty:     "2.0",
					Time:    1700001000000 + int64(i)*1000,
					IsMaker: true,
				}
			}
			json.NewEncoder(w).Encode(trades)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	client := newTestClient(srv.URL)

	writer, err := recorder.NewRotatingWriter(filepath.Join(dir, "trades"), "trade")
	if err != nil {
		t.Fatal(err)
	}
	cp, err := NewCheckpointStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	fetcher := NewTradeFetcher(client, writer, cp, slog.Default())

	n, err := fetcher.Fetch(context.Background(), "SOLUSDT", TimeRange{
		Start: time.UnixMilli(1700000000000),
		End:   time.UnixMilli(1700002000000),
	})
	writer.Close()

	if err != nil {
		t.Fatal(err)
	}
	expected := int64(maxTradesPerReq + 5)
	if n != expected {
		t.Errorf("expected %d records, got %d", expected, n)
	}

	// Verify checkpoint was saved
	saved, err := cp.Load("SOLUSDT", "trades")
	if err != nil {
		t.Fatal(err)
	}
	if saved.LastTS == 0 {
		t.Error("expected checkpoint to be saved")
	}
}

func TestTradeFetcher_Resume(t *testing.T) {
	// Server returns 3 trades starting from the requested startTime
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trades := []RawTrade{
			{ID: 10, Price: "100.00", Qty: "1.0", Time: 1700000500000, IsMaker: false},
			{ID: 11, Price: "100.50", Qty: "1.5", Time: 1700000501000, IsMaker: true},
			{ID: 12, Price: "101.00", Qty: "2.0", Time: 1700000502000, IsMaker: false},
		}
		json.NewEncoder(w).Encode(trades)
	}))
	defer srv.Close()

	dir := t.TempDir()
	client := newTestClient(srv.URL)

	// Pre-save a checkpoint to simulate a resume scenario
	cp, err := NewCheckpointStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = cp.Save(Checkpoint{Symbol: "SOLUSDT", DataType: "trades", LastTS: 1700000499999})

	writer, err := recorder.NewRotatingWriter(filepath.Join(dir, "trades"), "trade")
	if err != nil {
		t.Fatal(err)
	}
	fetcher := NewTradeFetcher(client, writer, cp, slog.Default())

	n, err := fetcher.Fetch(context.Background(), "SOLUSDT", TimeRange{
		Start: time.UnixMilli(1700000000000),
		End:   time.UnixMilli(1700001000000),
	})
	writer.Close()

	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("expected 3 records after resume, got %d", n)
	}
}

func TestTradeFetcher_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]RawTrade{})
	}))
	defer srv.Close()

	dir := t.TempDir()
	client := newTestClient(srv.URL)

	writer, err := recorder.NewRotatingWriter(filepath.Join(dir, "trades"), "trade")
	if err != nil {
		t.Fatal(err)
	}
	cp, err := NewCheckpointStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	fetcher := NewTradeFetcher(client, writer, cp, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, fetchErr := fetcher.Fetch(ctx, "SOLUSDT", TimeRange{
		Start: time.UnixMilli(1700000000000),
		End:   time.UnixMilli(1700001000000),
	})
	writer.Close()

	if fetchErr == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestKlineFetcher(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		w.Header().Set("Content-Type", "application/json")

		if page == 1 {
			// Return 3 klines (partial page = end of data)
			klines := make([][]interface{}, 3)
			for i := range klines {
				openTime := 1700000000000 + int64(i)*60000
				klines[i] = []interface{}{
					openTime,
					"100.50", "101.00", "100.00", "100.80", "1500.5",
					openTime + 59999,
					"150050.0", 320, "800.0", "80200.0", "0",
				}
			}
			json.NewEncoder(w).Encode(klines)
		} else {
			json.NewEncoder(w).Encode([][]interface{}{})
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	client := newTestClient(srv.URL)

	writer, err := recorder.NewRotatingWriter(filepath.Join(dir, "kiyotaka"), "kiyotaka")
	if err != nil {
		t.Fatal(err)
	}
	cp, err := NewCheckpointStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	fetcher := NewKlineFetcher(client, writer, cp, slog.Default())

	n, err := fetcher.Fetch(context.Background(), "SOLUSDT", TimeRange{
		Start: time.UnixMilli(1700000000000),
		End:   time.UnixMilli(1700000300000),
	})
	writer.Close()

	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("expected 3 kline records, got %d", n)
	}

	// Verify checkpoint
	saved, err := cp.Load("SOLUSDT", "klines")
	if err != nil {
		t.Fatal(err)
	}
	if saved.LastTS == 0 {
		t.Error("expected checkpoint to be saved")
	}
}

func TestKlineFetcher_MultiPage(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		w.Header().Set("Content-Type", "application/json")

		if page == 1 {
			// Return maxKlinesPerReq klines (full page)
			klines := make([][]interface{}, maxKlinesPerReq)
			for i := range klines {
				openTime := 1700000000000 + int64(i)*60000
				klines[i] = []interface{}{
					openTime,
					"100.50", "101.00", "100.00", "100.80", "1500.5",
					openTime + 59999,
					"150050.0", 320, "800.0", "80200.0", "0",
				}
			}
			json.NewEncoder(w).Encode(klines)
		} else {
			// Return 2 klines (partial page)
			klines := make([][]interface{}, 2)
			baseTime := int64(1700000000000) + int64(maxKlinesPerReq)*60000 + 60001
			for i := range klines {
				openTime := baseTime + int64(i)*60000
				klines[i] = []interface{}{
					openTime,
					"102.00", "103.00", "101.50", "102.50", "2000.0",
					openTime + 59999,
					"204000.0", 400, "1000.0", "102000.0", "0",
				}
			}
			json.NewEncoder(w).Encode(klines)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	client := newTestClient(srv.URL)

	writer, err := recorder.NewRotatingWriter(filepath.Join(dir, "kiyotaka"), "kiyotaka")
	if err != nil {
		t.Fatal(err)
	}
	cp, err := NewCheckpointStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	fetcher := NewKlineFetcher(client, writer, cp, slog.Default())

	n, err := fetcher.Fetch(context.Background(), "SOLUSDT", TimeRange{
		Start: time.UnixMilli(1700000000000),
		End:   time.UnixMilli(1700200000000), // far enough in the future
	})
	writer.Close()

	if err != nil {
		t.Fatal(err)
	}
	expected := int64(maxKlinesPerReq + 2)
	if n != expected {
		t.Errorf("expected %d kline records, got %d", expected, n)
	}
}

func TestFundingFetcher(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fapi/v1/fundingRate" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		page++
		if page == 1 {
			// Return 3 funding records (partial page = end of data)
			funding := []RawFunding{
				{Symbol: "SOLUSDT", FundingRate: "0.00010000", FundingTime: 1700006400000},
				{Symbol: "SOLUSDT", FundingRate: "-0.00005000", FundingTime: 1700035200000},
				{Symbol: "SOLUSDT", FundingRate: "0.00020000", FundingTime: 1700064000000},
			}
			json.NewEncoder(w).Encode(funding)
		} else {
			json.NewEncoder(w).Encode([]RawFunding{})
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	client := newTestClient(srv.URL)

	writer, err := recorder.NewRotatingWriter(filepath.Join(dir, "kiyotaka"), "kiyotaka")
	if err != nil {
		t.Fatal(err)
	}
	cp, err := NewCheckpointStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	fetcher := NewFundingFetcher(client, writer, cp, slog.Default())

	n, err := fetcher.Fetch(context.Background(), "SOLUSDT", TimeRange{
		Start: time.UnixMilli(1700000000000),
		End:   time.UnixMilli(1700100000000),
	})
	writer.Close()

	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("expected 3 funding records, got %d", n)
	}

	// Verify checkpoint
	saved, err := cp.Load("SOLUSDT", "funding")
	if err != nil {
		t.Fatal(err)
	}
	if saved.LastTS == 0 {
		t.Error("expected checkpoint to be saved")
	}
}

func TestFundingFetcher_MultiPage(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		if page == 1 {
			// Return maxFundingPerReq records (full page)
			funding := make([]RawFunding, maxFundingPerReq)
			for i := range funding {
				funding[i] = RawFunding{
					Symbol:      "SOLUSDT",
					FundingRate: fmt.Sprintf("0.%08d", i),
					FundingTime: 1700000000000 + int64(i)*28800000, // 8 hours apart
				}
			}
			json.NewEncoder(w).Encode(funding)
		} else {
			// Return 2 records (partial page)
			funding := []RawFunding{
				{Symbol: "SOLUSDT", FundingRate: "0.00015000", FundingTime: 1700000000000 + int64(maxFundingPerReq)*28800000},
				{Symbol: "SOLUSDT", FundingRate: "0.00025000", FundingTime: 1700000000000 + int64(maxFundingPerReq+1)*28800000},
			}
			json.NewEncoder(w).Encode(funding)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	client := newTestClient(srv.URL)

	writer, err := recorder.NewRotatingWriter(filepath.Join(dir, "kiyotaka"), "kiyotaka")
	if err != nil {
		t.Fatal(err)
	}
	cp, err := NewCheckpointStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	fetcher := NewFundingFetcher(client, writer, cp, slog.Default())

	n, err := fetcher.Fetch(context.Background(), "SOLUSDT", TimeRange{
		Start: time.UnixMilli(1700000000000),
		End:   time.UnixMilli(1800000000000), // far future
	})
	writer.Close()

	if err != nil {
		t.Fatal(err)
	}
	expected := int64(maxFundingPerReq + 2)
	if n != expected {
		t.Errorf("expected %d funding records, got %d", expected, n)
	}
}

func TestOIFetcher(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/futures/data/openInterestHist" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		page++
		if page == 1 {
			// Return 4 OI records (partial page)
			oi := []RawOI{
				{Symbol: "SOLUSDT", SumOpenInterest: "5000000.00", SumOpenInterestValue: "100000000.00", Timestamp: 1700000000000},
				{Symbol: "SOLUSDT", SumOpenInterest: "5100000.00", SumOpenInterestValue: "102000000.00", Timestamp: 1700000300000},
				{Symbol: "SOLUSDT", SumOpenInterest: "4900000.00", SumOpenInterestValue: "98000000.00", Timestamp: 1700000600000},
				{Symbol: "SOLUSDT", SumOpenInterest: "5200000.00", SumOpenInterestValue: "104000000.00", Timestamp: 1700000900000},
			}
			json.NewEncoder(w).Encode(oi)
		} else {
			json.NewEncoder(w).Encode([]RawOI{})
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	client := newTestClient(srv.URL)

	writer, err := recorder.NewRotatingWriter(filepath.Join(dir, "kiyotaka"), "kiyotaka")
	if err != nil {
		t.Fatal(err)
	}
	cp, err := NewCheckpointStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	fetcher := NewOIFetcher(client, writer, cp, slog.Default())

	n, err := fetcher.Fetch(context.Background(), "SOLUSDT", TimeRange{
		Start: time.UnixMilli(1700000000000),
		End:   time.UnixMilli(1700001000000),
	})
	writer.Close()

	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Errorf("expected 4 OI records, got %d", n)
	}

	// Verify checkpoint
	saved, err := cp.Load("SOLUSDT", "oi")
	if err != nil {
		t.Fatal(err)
	}
	if saved.LastTS == 0 {
		t.Error("expected checkpoint to be saved")
	}
}

func TestOIFetcher_MultiPage(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		if page == 1 {
			// Return maxOIPerReq records (full page)
			oi := make([]RawOI, maxOIPerReq)
			for i := range oi {
				oi[i] = RawOI{
					Symbol:               "SOLUSDT",
					SumOpenInterest:      fmt.Sprintf("%d.00", 5000000+i),
					SumOpenInterestValue: fmt.Sprintf("%d.00", 100000000+i*2000),
					Timestamp:            1700000000000 + int64(i)*300000, // 5 minutes apart
				}
			}
			json.NewEncoder(w).Encode(oi)
		} else {
			// Return 3 records (partial page)
			oi := []RawOI{
				{Symbol: "SOLUSDT", SumOpenInterest: "6000000.00", SumOpenInterestValue: "120000000.00", Timestamp: 1700000000000 + int64(maxOIPerReq)*300000},
				{Symbol: "SOLUSDT", SumOpenInterest: "6100000.00", SumOpenInterestValue: "122000000.00", Timestamp: 1700000000000 + int64(maxOIPerReq+1)*300000},
				{Symbol: "SOLUSDT", SumOpenInterest: "6200000.00", SumOpenInterestValue: "124000000.00", Timestamp: 1700000000000 + int64(maxOIPerReq+2)*300000},
			}
			json.NewEncoder(w).Encode(oi)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	client := newTestClient(srv.URL)

	writer, err := recorder.NewRotatingWriter(filepath.Join(dir, "kiyotaka"), "kiyotaka")
	if err != nil {
		t.Fatal(err)
	}
	cp, err := NewCheckpointStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	fetcher := NewOIFetcher(client, writer, cp, slog.Default())

	n, err := fetcher.Fetch(context.Background(), "SOLUSDT", TimeRange{
		Start: time.UnixMilli(1700000000000),
		End:   time.UnixMilli(1800000000000), // far future
	})
	writer.Close()

	if err != nil {
		t.Fatal(err)
	}
	expected := int64(maxOIPerReq + 3)
	if n != expected {
		t.Errorf("expected %d OI records, got %d", expected, n)
	}
}

func TestTradeFetcher_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]RawTrade{})
	}))
	defer srv.Close()

	dir := t.TempDir()
	client := newTestClient(srv.URL)

	writer, err := recorder.NewRotatingWriter(filepath.Join(dir, "trades"), "trade")
	if err != nil {
		t.Fatal(err)
	}
	cp, err := NewCheckpointStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	fetcher := NewTradeFetcher(client, writer, cp, slog.Default())

	n, err := fetcher.Fetch(context.Background(), "SOLUSDT", TimeRange{
		Start: time.UnixMilli(1700000000000),
		End:   time.UnixMilli(1700001000000),
	})
	writer.Close()

	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("expected 0 records for empty response, got %d", n)
	}
}

func TestTradeFetcher_IsBuyMapping(t *testing.T) {
	// Verify that IsMaker=true results in isBuy=false and vice versa
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trades := []RawTrade{
			{ID: 1, Price: "100.00", Qty: "1.0", Time: 1700000000000, IsMaker: true},  // seller is maker -> NOT a buy
			{ID: 2, Price: "100.00", Qty: "1.0", Time: 1700000001000, IsMaker: false}, // buyer is maker -> IS a buy
		}
		json.NewEncoder(w).Encode(trades)
	}))
	defer srv.Close()

	dir := t.TempDir()
	client := newTestClient(srv.URL)

	writer, err := recorder.NewRotatingWriter(filepath.Join(dir, "trades"), "trade")
	if err != nil {
		t.Fatal(err)
	}
	cp, err := NewCheckpointStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	fetcher := NewTradeFetcher(client, writer, cp, slog.Default())

	n, err := fetcher.Fetch(context.Background(), "SOLUSDT", TimeRange{
		Start: time.UnixMilli(1700000000000),
		End:   time.UnixMilli(1700001000000),
	})
	writer.Close()

	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("expected 2 records, got %d", n)
	}
}
