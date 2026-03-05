package models

import (
	"math"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func ptrFloat64(v float64) *float64 { return &v }
func ptrBool(v bool) *bool          { return &v }

func makeBid(price, size float64, entryID string) BookItem {
	return BookItem{
		IsBid:   true,
		Price:   ptrFloat64(price),
		Size:    ptrFloat64(size),
		EntryID: entryID,
	}
}

func makeAsk(price, size float64, entryID string) BookItem {
	return BookItem{
		IsBid:   false,
		Price:   ptrFloat64(price),
		Size:    ptrFloat64(size),
		EntryID: entryID,
	}
}

func makeDelta(isBid bool, price, size float64, entryID string) DeltaBookItem {
	return DeltaBookItem{
		IsBid:          ptrBool(isBid),
		Price:          ptrFloat64(price),
		Size:           ptrFloat64(size),
		EntryID:        entryID,
		LocalTimestamp:  time.Now(),
		ServerTimestamp: time.Now(),
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestOrderBook_LoadData(t *testing.T) {
	ob := NewOrderBook("BTCUSD", 2, 0)

	bids := []BookItem{
		makeBid(100.0, 10.0, "b1"),
		makeBid(102.0, 12.0, "b2"),
		makeBid(101.0, 11.0, "b3"),
		makeBid(99.0, 9.0, "b4"),
		makeBid(103.0, 13.0, "b5"),
	}
	asks := []BookItem{
		makeAsk(105.0, 15.0, "a1"),
		makeAsk(107.0, 17.0, "a2"),
		makeAsk(106.0, 16.0, "a3"),
		makeAsk(104.0, 14.0, "a4"),
		makeAsk(108.0, 18.0, "a5"),
	}

	ok := ob.LoadData(asks, bids)
	if !ok {
		t.Fatal("LoadData should return true")
	}

	// Verify bids sorted descending
	gotBids := ob.Bids()
	if len(gotBids) != 5 {
		t.Fatalf("expected 5 bids, got %d", len(gotBids))
	}
	expectedBidPrices := []float64{103.0, 102.0, 101.0, 100.0, 99.0}
	for i, exp := range expectedBidPrices {
		if *gotBids[i].Price != exp {
			t.Errorf("bid[%d] expected price %f, got %f", i, exp, *gotBids[i].Price)
		}
	}

	// Verify asks sorted ascending
	gotAsks := ob.Asks()
	if len(gotAsks) != 5 {
		t.Fatalf("expected 5 asks, got %d", len(gotAsks))
	}
	expectedAskPrices := []float64{104.0, 105.0, 106.0, 107.0, 108.0}
	for i, exp := range expectedAskPrices {
		if *gotAsks[i].Price != exp {
			t.Errorf("ask[%d] expected price %f, got %f", i, exp, *gotAsks[i].Price)
		}
	}

	// LoadData with empty should return false
	ok = ob.LoadData(nil, nil)
	if ok {
		t.Error("LoadData with nil should return false")
	}
}

func TestOrderBook_MidPrice(t *testing.T) {
	ob := NewOrderBook("BTCUSD", 2, 0)
	bids := []BookItem{makeBid(100.0, 10.0, "b1")}
	asks := []BookItem{makeAsk(102.0, 10.0, "a1")}
	ob.LoadData(asks, bids)

	mid := ob.MidPrice()
	expected := (100.0 + 102.0) / 2.0
	if mid != expected {
		t.Errorf("expected MidPrice %f, got %f", expected, mid)
	}
}

func TestOrderBook_Spread(t *testing.T) {
	ob := NewOrderBook("BTCUSD", 2, 0)
	bids := []BookItem{makeBid(100.0, 10.0, "b1")}
	asks := []BookItem{makeAsk(102.0, 10.0, "a1")}
	ob.LoadData(asks, bids)

	spread := ob.Spread()
	expected := 102.0 - 100.0
	if spread != expected {
		t.Errorf("expected Spread %f, got %f", expected, spread)
	}
}

func TestOrderBook_ImbalanceValue(t *testing.T) {
	ob := NewOrderBook("BTCUSD", 2, 0)

	// Bid sizes: 10 + 20 = 30, Ask sizes: 5 + 5 = 10
	// Imbalance = (30 - 10) / (30 + 10) = 20/40 = 0.5
	bids := []BookItem{
		makeBid(100.0, 10.0, "b1"),
		makeBid(99.0, 20.0, "b2"),
	}
	asks := []BookItem{
		makeAsk(101.0, 5.0, "a1"),
		makeAsk(102.0, 5.0, "a2"),
	}
	ob.LoadData(asks, bids)

	imb := ob.ImbalanceValue()
	if math.Abs(imb-0.5) > 1e-9 {
		t.Errorf("expected ImbalanceValue 0.5, got %f", imb)
	}
}

func TestOrderBook_AddLevel(t *testing.T) {
	ob := NewOrderBook("BTCUSD", 2, 0)
	ob.LoadData(
		[]BookItem{makeAsk(101.0, 5.0, "a1")},
		[]BookItem{makeBid(100.0, 10.0, "b1")},
	)

	// Add a bid at 99.5 (should go after 100.0)
	ob.AddLevel(makeDelta(true, 99.5, 8.0, "b2"))

	gotBids := ob.Bids()
	if len(gotBids) != 2 {
		t.Fatalf("expected 2 bids, got %d", len(gotBids))
	}
	if *gotBids[0].Price != 100.0 {
		t.Errorf("bid[0] expected 100.0, got %f", *gotBids[0].Price)
	}
	if *gotBids[1].Price != 99.5 {
		t.Errorf("bid[1] expected 99.5, got %f", *gotBids[1].Price)
	}

	// Add a bid at 100.5 (should go before 100.0)
	ob.AddLevel(makeDelta(true, 100.5, 12.0, "b3"))
	gotBids = ob.Bids()
	if len(gotBids) != 3 {
		t.Fatalf("expected 3 bids, got %d", len(gotBids))
	}
	if *gotBids[0].Price != 100.5 {
		t.Errorf("bid[0] expected 100.5, got %f", *gotBids[0].Price)
	}

	// Add an ask at 101.5 (should go after 101.0)
	ob.AddLevel(makeDelta(false, 101.5, 6.0, "a2"))
	gotAsks := ob.Asks()
	if len(gotAsks) != 2 {
		t.Fatalf("expected 2 asks, got %d", len(gotAsks))
	}
	if *gotAsks[0].Price != 101.0 {
		t.Errorf("ask[0] expected 101.0, got %f", *gotAsks[0].Price)
	}
	if *gotAsks[1].Price != 101.5 {
		t.Errorf("ask[1] expected 101.5, got %f", *gotAsks[1].Price)
	}
}

func TestOrderBook_UpdateLevel(t *testing.T) {
	ob := NewOrderBook("BTCUSD", 2, 0)
	ob.LoadData(
		[]BookItem{makeAsk(101.0, 5.0, "a1")},
		[]BookItem{makeBid(100.0, 10.0, "b1")},
	)

	// Update bid b1 size from 10 to 25
	ob.UpdateLevel(makeDelta(true, 100.0, 25.0, "b1"))

	gotBids := ob.Bids()
	if *gotBids[0].Size != 25.0 {
		t.Errorf("expected bid size 25.0, got %f", *gotBids[0].Size)
	}

	// Update by price match (no entryID)
	ob.UpdateLevel(DeltaBookItem{
		IsBid:          ptrBool(false),
		Price:          ptrFloat64(101.0),
		Size:           ptrFloat64(50.0),
		LocalTimestamp:  time.Now(),
		ServerTimestamp: time.Now(),
	})
	gotAsks := ob.Asks()
	if *gotAsks[0].Size != 50.0 {
		t.Errorf("expected ask size 50.0, got %f", *gotAsks[0].Size)
	}
}

func TestOrderBook_DeleteLevel(t *testing.T) {
	ob := NewOrderBook("BTCUSD", 2, 0)
	ob.LoadData(
		[]BookItem{
			makeAsk(101.0, 5.0, "a1"),
			makeAsk(102.0, 6.0, "a2"),
		},
		[]BookItem{
			makeBid(100.0, 10.0, "b1"),
			makeBid(99.0, 9.0, "b2"),
		},
	)

	// Delete bid b1
	ob.DeleteLevel(makeDelta(true, 100.0, 0, "b1"))

	gotBids := ob.Bids()
	if len(gotBids) != 1 {
		t.Fatalf("expected 1 bid after delete, got %d", len(gotBids))
	}
	if *gotBids[0].Price != 99.0 {
		t.Errorf("remaining bid should be 99.0, got %f", *gotBids[0].Price)
	}

	// Delete ask a2 by price match (empty entryID)
	ob.DeleteLevel(DeltaBookItem{
		IsBid:          ptrBool(false),
		Price:          ptrFloat64(102.0),
		LocalTimestamp:  time.Now(),
		ServerTimestamp: time.Now(),
	})
	gotAsks := ob.Asks()
	if len(gotAsks) != 1 {
		t.Fatalf("expected 1 ask after delete, got %d", len(gotAsks))
	}
	if *gotAsks[0].Price != 101.0 {
		t.Errorf("remaining ask should be 101.0, got %f", *gotAsks[0].Price)
	}
}

func TestOrderBook_AddOrUpdateLevel_Add(t *testing.T) {
	ob := NewOrderBook("BTCUSD", 2, 0)
	ob.LoadData(
		[]BookItem{makeAsk(101.0, 5.0, "a1")},
		[]BookItem{makeBid(100.0, 10.0, "b1")},
	)

	// AddOrUpdate a new bid (should add)
	ob.AddOrUpdateLevel(makeDelta(true, 99.0, 7.0, "b_new"))

	gotBids := ob.Bids()
	if len(gotBids) != 2 {
		t.Fatalf("expected 2 bids, got %d", len(gotBids))
	}
	if *gotBids[1].Price != 99.0 {
		t.Errorf("new bid should be at index 1 with price 99.0, got %f", *gotBids[1].Price)
	}
}

func TestOrderBook_AddOrUpdateLevel_Update(t *testing.T) {
	ob := NewOrderBook("BTCUSD", 2, 0)
	ob.LoadData(
		[]BookItem{makeAsk(101.0, 5.0, "a1")},
		[]BookItem{makeBid(100.0, 10.0, "b1")},
	)

	// AddOrUpdate existing bid b1 (should update)
	ob.AddOrUpdateLevel(makeDelta(true, 100.0, 20.0, "b1"))

	gotBids := ob.Bids()
	if len(gotBids) != 1 {
		t.Fatalf("expected 1 bid (updated not added), got %d", len(gotBids))
	}
	if *gotBids[0].Size != 20.0 {
		t.Errorf("expected updated size 20.0, got %f", *gotBids[0].Size)
	}
}

func TestOrderBook_MaxDepth(t *testing.T) {
	ob := NewOrderBook("BTCUSD", 2, 3) // MaxDepth = 3

	// Add 5 bid levels
	for i := 0; i < 5; i++ {
		price := 100.0 + float64(i)
		ob.AddLevel(makeDelta(true, price, 10.0, ""))
	}

	// Should only keep best 3 (highest prices for bids)
	gotBids := ob.Bids()
	if len(gotBids) != 3 {
		t.Fatalf("expected 3 bids (MaxDepth), got %d", len(gotBids))
	}
	// Best bids: 104, 103, 102
	expectedPrices := []float64{104.0, 103.0, 102.0}
	for i, exp := range expectedPrices {
		if *gotBids[i].Price != exp {
			t.Errorf("bid[%d] expected %f, got %f", i, exp, *gotBids[i].Price)
		}
	}

	// Add 5 ask levels
	for i := 0; i < 5; i++ {
		price := 105.0 + float64(i)
		ob.AddLevel(makeDelta(false, price, 10.0, ""))
	}

	// Should only keep best 3 (lowest prices for asks)
	gotAsks := ob.Asks()
	if len(gotAsks) != 3 {
		t.Fatalf("expected 3 asks (MaxDepth), got %d", len(gotAsks))
	}
	// Best asks: 105, 106, 107
	expectedAskPrices := []float64{105.0, 106.0, 107.0}
	for i, exp := range expectedAskPrices {
		if *gotAsks[i].Price != exp {
			t.Errorf("ask[%d] expected %f, got %f", i, exp, *gotAsks[i].Price)
		}
	}
}

func TestOrderBook_GetCounters(t *testing.T) {
	ob := NewOrderBook("BTCUSD", 2, 0)

	// Add 2 levels
	ob.AddLevel(makeDelta(true, 100.0, 10.0, "b1"))
	ob.AddLevel(makeDelta(false, 101.0, 5.0, "a1"))

	added, deleted, updated := ob.GetCounters()
	if added != 2 {
		t.Errorf("expected added=2, got %d", added)
	}
	if deleted != 0 {
		t.Errorf("expected deleted=0, got %d", deleted)
	}
	if updated != 0 {
		t.Errorf("expected updated=0, got %d", updated)
	}

	// Update 1 level
	ob.UpdateLevel(makeDelta(true, 100.0, 20.0, "b1"))
	_, _, updated = ob.GetCounters()
	if updated != 1 {
		t.Errorf("expected updated=1, got %d", updated)
	}

	// Delete 1 level
	ob.DeleteLevel(makeDelta(false, 101.0, 0, "a1"))
	_, deleted, _ = ob.GetCounters()
	if deleted != 1 {
		t.Errorf("expected deleted=1, got %d", deleted)
	}

	// Reset and verify
	ob.ResetCounters()
	added, deleted, updated = ob.GetCounters()
	if added != 0 || deleted != 0 || updated != 0 {
		t.Errorf("after reset, expected all counters 0, got added=%d deleted=%d updated=%d",
			added, deleted, updated)
	}
}

func TestOrderBook_Clone(t *testing.T) {
	ob := NewOrderBook("BTCUSD", 2, 0)
	ob.ProviderID = 1
	ob.ProviderName = "Test"
	ob.LoadData(
		[]BookItem{makeAsk(101.0, 5.0, "a1"), makeAsk(102.0, 6.0, "a2")},
		[]BookItem{makeBid(100.0, 10.0, "b1"), makeBid(99.0, 9.0, "b2")},
	)

	clone := ob.Clone()

	// Verify clone has same data
	if clone.Symbol != ob.Symbol {
		t.Errorf("clone symbol mismatch")
	}
	if clone.ProviderID != ob.ProviderID {
		t.Errorf("clone providerID mismatch")
	}
	if clone.BidCount() != ob.BidCount() {
		t.Errorf("clone bid count mismatch")
	}
	if clone.AskCount() != ob.AskCount() {
		t.Errorf("clone ask count mismatch")
	}
	if clone.MidPrice() != ob.MidPrice() {
		t.Errorf("clone midprice mismatch")
	}

	// Modify original, verify clone unchanged
	ob.AddLevel(makeDelta(true, 98.0, 8.0, "b3"))
	ob.Symbol = "MODIFIED"

	if clone.BidCount() != 2 {
		t.Errorf("clone bid count should still be 2, got %d", clone.BidCount())
	}
	if clone.Symbol != "BTCUSD" {
		t.Errorf("clone symbol should still be BTCUSD, got %s", clone.Symbol)
	}
}

func TestOrderBook_GetTOB(t *testing.T) {
	ob := NewOrderBook("BTCUSD", 2, 0)
	ob.LoadData(
		[]BookItem{
			makeAsk(102.0, 6.0, "a1"),
			makeAsk(101.0, 5.0, "a2"),
		},
		[]BookItem{
			makeBid(100.0, 10.0, "b1"),
			makeBid(99.0, 9.0, "b2"),
		},
	)

	// Best bid should be 100.0
	tob := ob.GetTOB(true)
	if tob == nil {
		t.Fatal("expected non-nil bid TOB")
	}
	if *tob.Price != 100.0 {
		t.Errorf("expected best bid 100.0, got %f", *tob.Price)
	}

	// Best ask should be 101.0
	tob = ob.GetTOB(false)
	if tob == nil {
		t.Fatal("expected non-nil ask TOB")
	}
	if *tob.Price != 101.0 {
		t.Errorf("expected best ask 101.0, got %f", *tob.Price)
	}

	// Verify returned value is a copy (modifying it should not affect book)
	*tob.Price = 999.0
	tobAgain := ob.GetTOB(false)
	if *tobAgain.Price != 101.0 {
		t.Errorf("GetTOB should return a copy, original was modified")
	}
}

func TestOrderBook_EmptyBook(t *testing.T) {
	ob := NewOrderBook("BTCUSD", 2, 0)

	if ob.MidPrice() != 0 {
		t.Errorf("empty book MidPrice should be 0, got %f", ob.MidPrice())
	}
	if ob.Spread() != 0 {
		t.Errorf("empty book Spread should be 0, got %f", ob.Spread())
	}
	if ob.ImbalanceValue() != 0 {
		t.Errorf("empty book ImbalanceValue should be 0, got %f", ob.ImbalanceValue())
	}
	if ob.GetTOB(true) != nil {
		t.Error("empty book bid TOB should be nil")
	}
	if ob.GetTOB(false) != nil {
		t.Error("empty book ask TOB should be nil")
	}
	if ob.GetMaxOrderSize() != 0 {
		t.Errorf("empty book GetMaxOrderSize should be 0, got %f", ob.GetMaxOrderSize())
	}
	if ob.BidCount() != 0 {
		t.Errorf("empty book BidCount should be 0, got %d", ob.BidCount())
	}
	if ob.AskCount() != 0 {
		t.Errorf("empty book AskCount should be 0, got %d", ob.AskCount())
	}
}

func TestOrderBook_ThreadSafety(t *testing.T) {
	ob := NewOrderBook("BTCUSD", 2, 100)

	// Pre-populate some data
	bids := make([]BookItem, 10)
	asks := make([]BookItem, 10)
	for i := 0; i < 10; i++ {
		bids[i] = makeBid(100.0-float64(i), float64(10+i), "")
		asks[i] = makeAsk(101.0+float64(i), float64(10+i), "")
	}
	ob.LoadData(asks, bids)

	var wg sync.WaitGroup
	const goroutines = 20
	const iterations = 200

	// Writers: add/update/delete levels concurrently
	for g := 0; g < goroutines/2; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				price := 50.0 + float64(i%50) + float64(id)*0.01
				switch i % 4 {
				case 0:
					ob.AddLevel(makeDelta(true, price, float64(i+1), ""))
				case 1:
					ob.AddOrUpdateLevel(makeDelta(false, price+50, float64(i+1), ""))
				case 2:
					ob.UpdateLevel(makeDelta(true, price, float64(i+100), ""))
				case 3:
					ob.DeleteLevel(makeDelta(false, price+50, 0, ""))
				}
			}
		}(g)
	}

	// Readers: read data concurrently
	for g := 0; g < goroutines/2; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_ = ob.MidPrice()
				_ = ob.Spread()
				_ = ob.ImbalanceValue()
				_ = ob.Bids()
				_ = ob.Asks()
				_ = ob.GetTOB(true)
				_ = ob.GetTOB(false)
				_ = ob.GetMaxOrderSize()
				_ = ob.BidCount()
				_ = ob.AskCount()
				ob.GetCounters()
			}
		}()
	}

	wg.Wait()
	// If we reach here without panic or race detector complaint, the test passes
}
