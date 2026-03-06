package pool

import (
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
)

// ---------------------------------------------------------------------------
// BookItem
// ---------------------------------------------------------------------------

func TestGetPutBookItem(t *testing.T) {
	item := GetBookItem()
	if item == nil {
		t.Fatal("GetBookItem returned nil")
	}

	// Populate fields
	price := 100.5
	size := 10.0
	item.Symbol = "BTCUSD"
	item.ProviderID = 1
	item.EntryID = "e1"
	item.Price = &price
	item.Size = &size
	item.IsBid = true

	// Return to pool
	PutBookItem(item)

	// Get again — may or may not be the same pointer
	item2 := GetBookItem()
	if item2 == nil {
		t.Fatal("GetBookItem returned nil after Put")
	}
}

func TestPoolReuseBookItem(t *testing.T) {
	item := GetBookItem()
	price := 42.0
	item.Symbol = "ETHUSD"
	item.Price = &price
	item.IsBid = true

	PutBookItem(item)

	// Immediately retrieve — likely the same object, which was reset
	item2 := GetBookItem()
	if item2.Symbol != "" {
		t.Errorf("expected Symbol to be reset, got %q", item2.Symbol)
	}
	if item2.Price != nil {
		t.Errorf("expected Price to be nil after reset, got %v", *item2.Price)
	}
	if item2.IsBid {
		t.Error("expected IsBid to be false after reset")
	}
}

// ---------------------------------------------------------------------------
// Trade
// ---------------------------------------------------------------------------

func TestGetPutTrade(t *testing.T) {
	tr := GetTrade()
	if tr == nil {
		t.Fatal("GetTrade returned nil")
	}

	// Populate fields
	isBuy := true
	tr.ProviderID = 7
	tr.Symbol = "AAPL"
	tr.Price = decimal.NewFromFloat(150.25)
	tr.Size = decimal.NewFromFloat(100)
	tr.Timestamp = time.Now()
	tr.IsBuy = &isBuy

	PutTrade(tr)

	tr2 := GetTrade()
	if tr2 == nil {
		t.Fatal("GetTrade returned nil after Put")
	}
}

func TestPoolReuseTrade(t *testing.T) {
	tr := GetTrade()
	isBuy := true
	tr.ProviderID = 3
	tr.Symbol = "GOOG"
	tr.Price = decimal.NewFromFloat(2800)
	tr.IsBuy = &isBuy

	PutTrade(tr)

	tr2 := GetTrade()
	if tr2.Symbol != "" {
		t.Errorf("expected Symbol to be reset, got %q", tr2.Symbol)
	}
	if tr2.ProviderID != 0 {
		t.Errorf("expected ProviderID to be 0 after reset, got %d", tr2.ProviderID)
	}
	if tr2.IsBuy != nil {
		t.Errorf("expected IsBuy to be nil after reset, got %v", *tr2.IsBuy)
	}
	if !tr2.Price.IsZero() {
		t.Errorf("expected Price to be zero after reset, got %s", tr2.Price)
	}
}

// ---------------------------------------------------------------------------
// OrderBook
// ---------------------------------------------------------------------------

func TestGetPutOrderBook(t *testing.T) {
	ob := GetOrderBook()
	if ob == nil {
		t.Fatal("GetOrderBook returned nil")
	}

	// Use the order book
	ob.Symbol = "BTCUSD"
	ob.ProviderID = 1
	ob.MaxDepth = 10

	PutOrderBook(ob)

	ob2 := GetOrderBook()
	if ob2 == nil {
		t.Fatal("GetOrderBook returned nil after Put")
	}
}

func TestPoolReuseOrderBook(t *testing.T) {
	ob := GetOrderBook()
	ob.Symbol = "ETHUSD"
	ob.ProviderID = 5

	price1 := 100.0
	size1 := 5.0
	ob.LoadData(
		[]models.BookItem{{Price: &price1, Size: &size1, IsBid: false}},
		[]models.BookItem{{Price: &price1, Size: &size1, IsBid: true}},
	)

	PutOrderBook(ob)

	ob2 := GetOrderBook()
	// After Reset, bids and asks should be empty
	if ob2.BidCount() != 0 {
		t.Errorf("expected 0 bids after reset, got %d", ob2.BidCount())
	}
	if ob2.AskCount() != 0 {
		t.Errorf("expected 0 asks after reset, got %d", ob2.AskCount())
	}
	added, deleted, updated := ob2.GetCounters()
	if added != 0 || deleted != 0 || updated != 0 {
		t.Errorf("expected counters to be 0 after reset, got added=%d deleted=%d updated=%d",
			added, deleted, updated)
	}
}

// ---------------------------------------------------------------------------
// Nil safety
// ---------------------------------------------------------------------------

func TestPutNil(t *testing.T) {
	// These should not panic
	PutBookItem(nil)
	PutTrade(nil)
	PutOrderBook(nil)
}

// ---------------------------------------------------------------------------
// Concurrent usage
// ---------------------------------------------------------------------------

func TestConcurrentPool(t *testing.T) {
	const goroutines = 100
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(goroutines * 3) // 3 pool types

	// BookItem concurrency
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				item := GetBookItem()
				price := float64(j)
				item.Price = &price
				item.Symbol = "TEST"
				PutBookItem(item)
			}
		}()
	}

	// Trade concurrency
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				tr := GetTrade()
				tr.Symbol = "TEST"
				tr.Price = decimal.NewFromInt(int64(j))
				PutTrade(tr)
			}
		}()
	}

	// OrderBook concurrency
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				ob := GetOrderBook()
				ob.Symbol = "TEST"
				PutOrderBook(ob)
			}
		}()
	}

	wg.Wait()
}
