package models

import (
	"encoding/json"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/core/enums"
)

// OrderBook manages a thread-safe, sorted limit order book with bid and ask
// levels. Bids are sorted by price descending (best bid first), asks are
// sorted by price ascending (best ask first).
type OrderBook struct {
	Symbol             string              `json:"symbol"`
	MaxDepth           int                 `json:"maxDepth"`
	PriceDecimalPlaces int                 `json:"priceDecimalPlaces"`
	SizeDecimalPlaces  int                 `json:"sizeDecimalPlaces"`
	SymbolMultiplier   float64             `json:"symbolMultiplier"`
	ProviderID         int                 `json:"providerId"`
	ProviderName       string              `json:"providerName"`
	ProviderStatus     enums.SessionStatus `json:"providerStatus"`
	Sequence           int64               `json:"sequence"`
	LastUpdated        *time.Time          `json:"lastUpdated,omitempty"`

	// Internal state (not exported in JSON)
	bids []BookItem // sorted by price DESCENDING (highest first)
	asks []BookItem // sorted by price ASCENDING (lowest first)
	mu   sync.RWMutex

	// Atomic counters for level changes
	addedLevels   atomic.Int64
	deletedLevels atomic.Int64
	updatedLevels atomic.Int64
}

// MarshalJSON implements json.Marshaler. It acquires the read lock and
// includes computed fields (midPrice, spread) and the private bids/asks
// slices so that JSON consumers (e.g. the React frontend) receive the
// full order book state.
func (ob *OrderBook) MarshalJSON() ([]byte, error) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	var midPrice, spread, microPrice float64
	if len(ob.bids) > 0 && len(ob.asks) > 0 {
		bestBid := priceOf(&ob.bids[0])
		bestAsk := priceOf(&ob.asks[0])
		midPrice = (bestBid + bestAsk) / 2
		spread = bestAsk - bestBid

		bidSize := sizeOf(&ob.bids[0])
		askSize := sizeOf(&ob.asks[0])
		total := bidSize + askSize
		if total > 0 {
			microPrice = (bestBid*askSize + bestAsk*bidSize) / total
		} else {
			microPrice = midPrice
		}
	}

	type Alias OrderBook // prevent infinite recursion
	return json.Marshal(&struct {
		Bids       []BookItem `json:"bids"`
		Asks       []BookItem `json:"asks"`
		MidPrice   float64    `json:"midPrice"`
		Spread     float64    `json:"spread"`
		MicroPrice float64    `json:"microPrice"`
		// Embed exported fields from OrderBook.
		Symbol             string              `json:"symbol"`
		MaxDepth           int                 `json:"maxDepth"`
		PriceDecimalPlaces int                 `json:"priceDecimalPlaces"`
		SizeDecimalPlaces  int                 `json:"sizeDecimalPlaces"`
		SymbolMultiplier   float64             `json:"symbolMultiplier"`
		ProviderID         int                 `json:"providerId"`
		ProviderName       string              `json:"providerName"`
		ProviderStatus     enums.SessionStatus `json:"providerStatus"`
		Sequence           int64               `json:"sequence"`
		LastUpdated        *time.Time          `json:"lastUpdated,omitempty"`
	}{
		Bids:               deepCopyLevels(ob.bids),
		Asks:               deepCopyLevels(ob.asks),
		MidPrice:           midPrice,
		Spread:             spread,
		MicroPrice:         microPrice,
		Symbol:             ob.Symbol,
		MaxDepth:           ob.MaxDepth,
		PriceDecimalPlaces: ob.PriceDecimalPlaces,
		SizeDecimalPlaces:  ob.SizeDecimalPlaces,
		SymbolMultiplier:   ob.SymbolMultiplier,
		ProviderID:         ob.ProviderID,
		ProviderName:       ob.ProviderName,
		ProviderStatus:     ob.ProviderStatus,
		Sequence:           ob.Sequence,
		LastUpdated:        ob.LastUpdated,
	})
}

// NewOrderBook creates a new OrderBook for the given symbol.
func NewOrderBook(symbol string, priceDecimalPlaces, maxDepth int) *OrderBook {
	return &OrderBook{
		Symbol:             symbol,
		PriceDecimalPlaces: priceDecimalPlaces,
		MaxDepth:           maxDepth,
		SymbolMultiplier:   1.0,
		bids:               make([]BookItem, 0),
		asks:               make([]BookItem, 0),
	}
}

// ---------------------------------------------------------------------------
// Thread-safe getters (acquire read lock)
// ---------------------------------------------------------------------------

// Bids returns a deep copy of bid levels (sorted price descending).
func (ob *OrderBook) Bids() []BookItem {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	return deepCopyLevels(ob.bids)
}

// Asks returns a deep copy of ask levels (sorted price ascending).
func (ob *OrderBook) Asks() []BookItem {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	return deepCopyLevels(ob.asks)
}

// MidPrice returns (bestBid + bestAsk) / 2, or 0 if either side is empty.
func (ob *OrderBook) MidPrice() float64 {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	if len(ob.bids) == 0 || len(ob.asks) == 0 {
		return 0
	}
	bestBid := priceOf(&ob.bids[0])
	bestAsk := priceOf(&ob.asks[0])
	return (bestBid + bestAsk) / 2
}

// MicroPrice returns the size-weighted mid-price:
//   (bestBid * askSize + bestAsk * bidSize) / (bidSize + askSize)
// This tilts the mid-price toward the side with less liquidity,
// providing a leading price signal. Returns 0 if either side is empty.
func (ob *OrderBook) MicroPrice() float64 {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	if len(ob.bids) == 0 || len(ob.asks) == 0 {
		return 0
	}
	bestBid := priceOf(&ob.bids[0])
	bestAsk := priceOf(&ob.asks[0])
	bidSize := sizeOf(&ob.bids[0])
	askSize := sizeOf(&ob.asks[0])
	total := bidSize + askSize
	if total == 0 {
		return (bestBid + bestAsk) / 2
	}
	return (bestBid*askSize + bestAsk*bidSize) / total
}

// Spread returns bestAsk - bestBid, or 0 if either side is empty.
func (ob *OrderBook) Spread() float64 {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	if len(ob.bids) == 0 || len(ob.asks) == 0 {
		return 0
	}
	bestBid := priceOf(&ob.bids[0])
	bestAsk := priceOf(&ob.asks[0])
	return bestAsk - bestBid
}

// ImbalanceValue returns (totalBidSize - totalAskSize) / (totalBidSize + totalAskSize).
// Range: [-1, 1]. Returns 0 if total volume is 0.
func (ob *OrderBook) ImbalanceValue() float64 {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	var totalBid, totalAsk float64
	for i := range ob.bids {
		totalBid += sizeOf(&ob.bids[i])
	}
	for i := range ob.asks {
		totalAsk += sizeOf(&ob.asks[i])
	}
	total := totalBid + totalAsk
	if total == 0 {
		return 0
	}
	return (totalBid - totalAsk) / total
}

// GetTOB returns top of book for the given side. Returns nil if empty.
// The returned BookItem is a deep copy.
func (ob *OrderBook) GetTOB(isBid bool) *BookItem {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	if isBid {
		if len(ob.bids) == 0 {
			return nil
		}
		var item BookItem
		item.CopyFrom(&ob.bids[0])
		return &item
	}
	if len(ob.asks) == 0 {
		return nil
	}
	var item BookItem
	item.CopyFrom(&ob.asks[0])
	return &item
}

// GetMaxOrderSize returns the largest size across all levels on both sides.
func (ob *OrderBook) GetMaxOrderSize() float64 {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	var maxSize float64
	for i := range ob.bids {
		if s := sizeOf(&ob.bids[i]); s > maxSize {
			maxSize = s
		}
	}
	for i := range ob.asks {
		if s := sizeOf(&ob.asks[i]); s > maxSize {
			maxSize = s
		}
	}
	return maxSize
}

// GetBidsSnapshot deep-copies bid levels into dst slice, returns count copied.
func (ob *OrderBook) GetBidsSnapshot(dst []BookItem) int {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	n := min(len(dst), len(ob.bids))
	for i := 0; i < n; i++ {
		dst[i].CopyFrom(&ob.bids[i])
	}
	return n
}

// GetAsksSnapshot deep-copies ask levels into dst slice, returns count copied.
func (ob *OrderBook) GetAsksSnapshot(dst []BookItem) int {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	n := min(len(dst), len(ob.asks))
	for i := 0; i < n; i++ {
		dst[i].CopyFrom(&ob.asks[i])
	}
	return n
}

// GetCounters returns level change counters since last reset.
func (ob *OrderBook) GetCounters() (added, deleted, updated int64) {
	return ob.addedLevels.Load(), ob.deletedLevels.Load(), ob.updatedLevels.Load()
}

// ResetCounters resets the level change counters to zero.
func (ob *OrderBook) ResetCounters() {
	ob.addedLevels.Store(0)
	ob.deletedLevels.Store(0)
	ob.updatedLevels.Store(0)
}

// BidCount returns number of bid levels.
func (ob *OrderBook) BidCount() int {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	return len(ob.bids)
}

// AskCount returns number of ask levels.
func (ob *OrderBook) AskCount() int {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	return len(ob.asks)
}

// ---------------------------------------------------------------------------
// Data loading (acquire write lock)
// ---------------------------------------------------------------------------

// LoadData replaces all bids and asks. Sorts them. Returns true if data was loaded.
func (ob *OrderBook) LoadData(asks, bids []BookItem) bool {
	if len(asks) == 0 && len(bids) == 0 {
		return false
	}

	ob.mu.Lock()
	defer ob.mu.Unlock()

	// Copy bids
	ob.bids = make([]BookItem, len(bids))
	copy(ob.bids, bids)

	// Copy asks
	ob.asks = make([]BookItem, len(asks))
	copy(ob.asks, asks)

	// Sort bids descending by price
	sortBidsDesc(ob.bids)
	// Sort asks ascending by price
	sortAsksAsc(ob.asks)

	// Trim to MaxDepth
	if ob.MaxDepth > 0 {
		if len(ob.bids) > ob.MaxDepth {
			ob.bids = ob.bids[:ob.MaxDepth]
		}
		if len(ob.asks) > ob.MaxDepth {
			ob.asks = ob.asks[:ob.MaxDepth]
		}
	}

	now := time.Now()
	ob.LastUpdated = &now
	return true
}

// AddOrUpdateLevel adds or updates a price level. If a level with the same
// EntryID or Price exists on the same side, it updates the size. Otherwise
// it inserts a new level at the correct sorted position.
func (ob *OrderBook) AddOrUpdateLevel(delta DeltaBookItem) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	isBid := delta.boolVal()
	side := ob.getSide(isBid)

	// Search for existing level
	idx := ob.findLevel(side, delta)
	if idx >= 0 {
		// Update existing
		ob.updateLevelAt(side, idx, delta)
		ob.updatedLevels.Add(1)
	} else {
		// Add new
		ob.insertLevel(isBid, delta)
		ob.addedLevels.Add(1)
	}

	// Trim if needed
	ob.trimSide(isBid)

	now := time.Now()
	ob.LastUpdated = &now
}

// AddLevel adds a new price level. Maintains sort order. Rejects if MaxDepth
// would be exceeded and the new level would be worse than the worst existing.
func (ob *OrderBook) AddLevel(delta DeltaBookItem) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	isBid := delta.boolVal()
	ob.insertLevel(isBid, delta)
	ob.addedLevels.Add(1)

	// Trim if needed
	ob.trimSide(isBid)

	now := time.Now()
	ob.LastUpdated = &now
}

// UpdateLevel updates an existing price level by EntryID or Price match.
func (ob *OrderBook) UpdateLevel(delta DeltaBookItem) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	isBid := delta.boolVal()
	side := ob.getSide(isBid)

	idx := ob.findLevel(side, delta)
	if idx >= 0 {
		ob.updateLevelAt(side, idx, delta)
		ob.updatedLevels.Add(1)

		now := time.Now()
		ob.LastUpdated = &now
	}
}

// DeleteLevel removes a price level by EntryID or Price match.
func (ob *OrderBook) DeleteLevel(delta DeltaBookItem) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	isBid := delta.boolVal()

	if isBid {
		idx := ob.findLevel(ob.bids, delta)
		if idx >= 0 {
			ob.bids = append(ob.bids[:idx], ob.bids[idx+1:]...)
			ob.deletedLevels.Add(1)

			now := time.Now()
			ob.LastUpdated = &now
		}
	} else {
		idx := ob.findLevel(ob.asks, delta)
		if idx >= 0 {
			ob.asks = append(ob.asks[:idx], ob.asks[idx+1:]...)
			ob.deletedLevels.Add(1)

			now := time.Now()
			ob.LastUpdated = &now
		}
	}
}

// Clear removes all levels.
func (ob *OrderBook) Clear() {
	ob.mu.Lock()
	defer ob.mu.Unlock()
	ob.bids = ob.bids[:0]
	ob.asks = ob.asks[:0]
}

// Reset clears levels, metadata, and counters. Safe for returning to a pool.
func (ob *OrderBook) Reset() {
	ob.mu.Lock()
	ob.Symbol = ""
	ob.MaxDepth = 0
	ob.PriceDecimalPlaces = 0
	ob.SizeDecimalPlaces = 0
	ob.SymbolMultiplier = 0
	ob.ProviderID = 0
	ob.ProviderName = ""
	ob.ProviderStatus = 0
	ob.Sequence = 0
	ob.LastUpdated = nil
	ob.bids = ob.bids[:0]
	ob.asks = ob.asks[:0]
	ob.mu.Unlock()

	ob.ResetCounters()
}

// ---------------------------------------------------------------------------
// Cloning
// ---------------------------------------------------------------------------

// Clone returns a deep copy of the order book.
func (ob *OrderBook) Clone() *OrderBook {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	clone := &OrderBook{
		Symbol:             ob.Symbol,
		MaxDepth:           ob.MaxDepth,
		PriceDecimalPlaces: ob.PriceDecimalPlaces,
		SizeDecimalPlaces:  ob.SizeDecimalPlaces,
		SymbolMultiplier:   ob.SymbolMultiplier,
		ProviderID:         ob.ProviderID,
		ProviderName:       ob.ProviderName,
		ProviderStatus:     ob.ProviderStatus,
		Sequence:           ob.Sequence,
		bids:               make([]BookItem, len(ob.bids)),
		asks:               make([]BookItem, len(ob.asks)),
	}

	if ob.LastUpdated != nil {
		t := *ob.LastUpdated
		clone.LastUpdated = &t
	}

	for i := range ob.bids {
		clone.bids[i].CopyFrom(&ob.bids[i])
	}
	for i := range ob.asks {
		clone.asks[i].CopyFrom(&ob.asks[i])
	}

	// Copy atomic counters
	clone.addedLevels.Store(ob.addedLevels.Load())
	clone.deletedLevels.Store(ob.deletedLevels.Load())
	clone.updatedLevels.Store(ob.updatedLevels.Load())

	return clone
}

// ---------------------------------------------------------------------------
// Internal helpers (caller must hold appropriate lock)
// ---------------------------------------------------------------------------

// getSide returns the slice for the given side. Note: the returned slice
// shares the underlying array with ob.bids or ob.asks.
func (ob *OrderBook) getSide(isBid bool) []BookItem {
	if isBid {
		return ob.bids
	}
	return ob.asks
}

// findLevel searches for a level matching the delta by EntryID (if non-empty)
// or by Price. Returns the index or -1 if not found.
func (ob *OrderBook) findLevel(side []BookItem, delta DeltaBookItem) int {
	if delta.EntryID != "" {
		for i := range side {
			if side[i].EntryID == delta.EntryID {
				return i
			}
		}
	}
	if delta.Price != nil {
		price := *delta.Price
		for i := range side {
			if side[i].Price != nil && *side[i].Price == price {
				return i
			}
		}
	}
	return -1
}

// updateLevelAt updates the level at the given index with data from delta.
func (ob *OrderBook) updateLevelAt(side []BookItem, idx int, delta DeltaBookItem) {
	if delta.Size != nil {
		v := *delta.Size
		side[idx].Size = &v
	}
	if delta.Price != nil {
		v := *delta.Price
		side[idx].Price = &v
	}
	side[idx].LocalTimestamp = delta.LocalTimestamp
	side[idx].ServerTimestamp = delta.ServerTimestamp
}

// insertLevel inserts a new level at the correct sorted position.
func (ob *OrderBook) insertLevel(isBid bool, delta DeltaBookItem) {
	item := BookItem{
		EntryID:        delta.EntryID,
		IsBid:          delta.boolVal(),
		LocalTimestamp:  delta.LocalTimestamp,
		ServerTimestamp: delta.ServerTimestamp,
	}
	if delta.Price != nil {
		v := *delta.Price
		item.Price = &v
	}
	if delta.Size != nil {
		v := *delta.Size
		item.Size = &v
	}

	if isBid {
		pos := ob.findInsertPosBid(delta.priceVal())
		ob.bids = append(ob.bids, BookItem{})
		copy(ob.bids[pos+1:], ob.bids[pos:])
		ob.bids[pos] = item
	} else {
		pos := ob.findInsertPosAsk(delta.priceVal())
		ob.asks = append(ob.asks, BookItem{})
		copy(ob.asks[pos+1:], ob.asks[pos:])
		ob.asks[pos] = item
	}
}

// findInsertPosBid returns the index where a bid with the given price
// should be inserted to maintain descending sort order.
func (ob *OrderBook) findInsertPosBid(price float64) int {
	return sort.Search(len(ob.bids), func(i int) bool {
		return priceOf(&ob.bids[i]) < price
	})
}

// findInsertPosAsk returns the index where an ask with the given price
// should be inserted to maintain ascending sort order.
func (ob *OrderBook) findInsertPosAsk(price float64) int {
	return sort.Search(len(ob.asks), func(i int) bool {
		return priceOf(&ob.asks[i]) > price
	})
}

// trimSide trims the given side to MaxDepth if it exceeds it.
// Worst levels are removed (last elements after sorting).
func (ob *OrderBook) trimSide(isBid bool) {
	if ob.MaxDepth <= 0 {
		return
	}
	if isBid {
		if len(ob.bids) > ob.MaxDepth {
			ob.bids = ob.bids[:ob.MaxDepth]
		}
	} else {
		if len(ob.asks) > ob.MaxDepth {
			ob.asks = ob.asks[:ob.MaxDepth]
		}
	}
}

// ---------------------------------------------------------------------------
// Sorting helpers
// ---------------------------------------------------------------------------

// sortBidsDesc sorts bids by price descending (highest first).
func sortBidsDesc(items []BookItem) {
	sort.Slice(items, func(i, j int) bool {
		return priceOf(&items[i]) > priceOf(&items[j])
	})
}

// sortAsksAsc sorts asks by price ascending (lowest first).
func sortAsksAsc(items []BookItem) {
	sort.Slice(items, func(i, j int) bool {
		return priceOf(&items[i]) < priceOf(&items[j])
	})
}

// priceOf returns the price of a BookItem, or 0 if Price is nil.
func priceOf(item *BookItem) float64 {
	if item.Price == nil {
		return 0
	}
	return *item.Price
}

// sizeOf returns the size of a BookItem, or 0 if Size is nil.
func sizeOf(item *BookItem) float64 {
	if item.Size == nil {
		return 0
	}
	return *item.Size
}

// deepCopyLevels returns a deep copy of a slice of BookItems.
func deepCopyLevels(src []BookItem) []BookItem {
	dst := make([]BookItem, len(src))
	for i := range src {
		dst[i].CopyFrom(&src[i])
	}
	return dst
}
