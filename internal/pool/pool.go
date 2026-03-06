// Package pool provides typed sync.Pool wrappers for frequently allocated
// domain objects, reducing GC pressure in hot paths.
package pool

import (
	"sync"

	"github.com/ashark-ai-05/tradefox/internal/core/models"
)

// ---------------------------------------------------------------------------
// BookItem pool
// ---------------------------------------------------------------------------

var bookItemPool = sync.Pool{
	New: func() any { return &models.BookItem{} },
}

// GetBookItem retrieves a BookItem from the pool (or creates a new one).
func GetBookItem() *models.BookItem {
	return bookItemPool.Get().(*models.BookItem)
}

// PutBookItem resets and returns a BookItem to the pool.
func PutBookItem(item *models.BookItem) {
	if item == nil {
		return
	}
	item.Reset()
	bookItemPool.Put(item)
}

// ---------------------------------------------------------------------------
// Trade pool
// ---------------------------------------------------------------------------

var tradePool = sync.Pool{
	New: func() any { return &models.Trade{} },
}

// GetTrade retrieves a Trade from the pool (or creates a new one).
func GetTrade() *models.Trade {
	return tradePool.Get().(*models.Trade)
}

// PutTrade zeroes out and returns a Trade to the pool.
func PutTrade(t *models.Trade) {
	if t == nil {
		return
	}
	*t = models.Trade{} // zero out
	tradePool.Put(t)
}

// ---------------------------------------------------------------------------
// OrderBook pool
// ---------------------------------------------------------------------------

var orderBookPool = sync.Pool{
	New: func() any { return models.NewOrderBook("", 0, 0) },
}

// GetOrderBook retrieves an OrderBook from the pool (or creates a new one).
func GetOrderBook() *models.OrderBook {
	return orderBookPool.Get().(*models.OrderBook)
}

// PutOrderBook resets and returns an OrderBook to the pool.
func PutOrderBook(ob *models.OrderBook) {
	if ob == nil {
		return
	}
	ob.Reset()
	orderBookPool.Put(ob)
}
