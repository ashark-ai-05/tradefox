package models

import (
	"sync"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/core/enums"
)

// Position tracks the aggregated state of buy and sell orders for a single
// symbol, including realized and unrealized P&L calculated via FIFO or LIFO
// matching. All mutating operations are thread-safe.
type Position struct {
	Symbol      string                   `json:"symbol"`
	TotBuy      float64                  `json:"totBuy"`
	TotSell     float64                  `json:"totSell"`
	WrkBuy      float64                  `json:"wrkBuy"`
	WrkSell     float64                  `json:"wrkSell"`
	PLTot       float64                  `json:"plTot"`
	PLRealized  float64                  `json:"plRealized"`
	PLOpen      float64                  `json:"plOpen"`
	LastUpdated time.Time                `json:"lastUpdated"`

	method          enums.PositionCalcMethod
	currentMidPrice float64
	buys            []Order
	sells           []Order
	mu              sync.RWMutex
}

// NewPosition creates a new Position for the given symbol and P&L calculation
// method (FIFO or LIFO).
func NewPosition(symbol string, method enums.PositionCalcMethod) *Position {
	return &Position{
		Symbol: symbol,
		method: method,
		buys:   make([]Order, 0),
		sells:  make([]Order, 0),
	}
}

// AddOrUpdateOrder adds a new order or updates an existing one. Thread-safe.
// Orders are classified as buys or sells based on order.Side. The position is
// recalculated after every add/update.
func (p *Position) AddOrUpdateOrder(order Order) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Try to find and update existing order by OrderID.
	if order.Side == enums.OrderSideBuy {
		for i := range p.buys {
			if p.buys[i].OrderID == order.OrderID {
				p.buys[i].Update(&order)
				p.recalculate()
				return
			}
		}
		p.buys = append(p.buys, order)
	} else {
		for i := range p.sells {
			if p.sells[i].OrderID == order.OrderID {
				p.sells[i].Update(&order)
				p.recalculate()
				return
			}
		}
		p.sells = append(p.sells, order)
	}

	p.recalculate()
}

// UpdateCurrentMidPrice updates the mid price used for open P&L calculation
// and recalculates. Returns true if the price changed.
func (p *Position) UpdateCurrentMidPrice(price float64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.currentMidPrice == price {
		return false
	}

	p.currentMidPrice = price
	p.recalculate()
	return true
}

// NetPosition returns TotBuy - TotSell (the net filled quantity).
func (p *Position) NetPosition() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.TotBuy - p.TotSell
}

// Exposure returns the net position valued at the current mid price.
func (p *Position) Exposure() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return (p.TotBuy - p.TotSell) * p.currentMidPrice
}

// GetAllOrders returns a copy of all buy and sell orders.
func (p *Position) GetAllOrders() []Order {
	p.mu.RLock()
	defer p.mu.RUnlock()

	all := make([]Order, 0, len(p.buys)+len(p.sells))
	all = append(all, p.buys...)
	all = append(all, p.sells...)
	return all
}

// recalculate computes TotBuy, TotSell, WrkBuy, WrkSell, PLRealized, PLOpen,
// and PLTot from the current buy and sell order lists. Must be called with
// p.mu held.
func (p *Position) recalculate() {
	p.TotBuy = 0
	p.TotSell = 0
	p.WrkBuy = 0
	p.WrkSell = 0

	for i := range p.buys {
		p.TotBuy += p.buys[i].FilledQuantity
		p.WrkBuy += p.buys[i].PendingQuantity()
	}

	for i := range p.sells {
		p.TotSell += p.sells[i].FilledQuantity
		p.WrkSell += p.sells[i].PendingQuantity()
	}

	p.PLRealized = CalculateRealizedPnL(p.buys, p.sells, p.method)
	p.PLOpen = CalculateOpenPnL(p.buys, p.sells, p.method, p.currentMidPrice)
	p.PLTot = p.PLRealized + p.PLOpen
	p.LastUpdated = time.Now()
}
