package views

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/persistence"
	"github.com/ashark-ai-05/tradefox/internal/tui/mock"
	"github.com/ashark-ai-05/tradefox/internal/tui/theme"
)

// PaperOrder represents a pending or filled paper order.
type PaperOrder struct {
	ID        int64
	Symbol    string
	Side      string // "BUY" or "SELL"
	Type      OrderType
	Price     float64 // limit/stop price
	Quantity  float64
	FilledAt  float64
	Status    string // "PENDING", "FILLED", "CANCELLED"
	CreatedAt time.Time
	FilledTime time.Time
}

// PaperPosition represents an open paper position.
type PaperPosition struct {
	Symbol   string
	Side     string // "LONG" or "SHORT"
	Size     float64
	Entry    float64
	Mark     float64
}

func (p PaperPosition) PnL() float64 {
	if p.Side == "LONG" {
		return (p.Mark - p.Entry) * p.Size
	}
	return (p.Entry - p.Mark) * p.Size
}

func (p PaperPosition) PnLPct() float64 {
	if p.Entry == 0 {
		return 0
	}
	if p.Side == "LONG" {
		return ((p.Mark - p.Entry) / p.Entry) * 100
	}
	return ((p.Entry - p.Mark) / p.Entry) * 100
}

// PaperEngine manages a virtual portfolio for paper trading.
type PaperEngine struct {
	mu            sync.RWMutex
	Active        bool
	Balance       float64
	StartBalance  float64
	Positions     []PaperPosition
	PendingOrders []PaperOrder
	OrderHistory  []PaperOrder
	nextOrderID   int64
	DB            *persistence.DB
	bestBid       float64
	bestAsk       float64
}

// NewPaperEngine creates a paper trading engine.
func NewPaperEngine(startingCapital float64) *PaperEngine {
	return &PaperEngine{
		Balance:      startingCapital,
		StartBalance: startingCapital,
		nextOrderID:  1,
	}
}

// SubmitOrder creates a new paper order.
func (pe *PaperEngine) SubmitOrder(symbol, side string, orderType OrderType, price, quantity float64) PaperOrder {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	order := PaperOrder{
		ID:        pe.nextOrderID,
		Symbol:    symbol,
		Side:      side,
		Type:      orderType,
		Price:     price,
		Quantity:  quantity,
		Status:    "PENDING",
		CreatedAt: time.Now(),
	}
	pe.nextOrderID++

	if orderType == OrderTypeMarket {
		// Fill at best bid (sell) or ask (buy) from live order book, or fallback to given price.
		fillPrice := price
		if side == "BUY" && pe.bestAsk > 0 {
			fillPrice = pe.bestAsk
		} else if side == "SELL" && pe.bestBid > 0 {
			fillPrice = pe.bestBid
		}
		order.FilledAt = fillPrice
		order.Status = "FILLED"
		order.FilledTime = time.Now()
		pe.applyFill(order)
		pe.OrderHistory = append(pe.OrderHistory, order)
	} else {
		pe.PendingOrders = append(pe.PendingOrders, order)
	}

	return order
}

// UpdatePrices checks pending orders against current prices and fills if triggered.
func (pe *PaperEngine) UpdatePrices(prices map[string]mock.WatchlistEntry) {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	// Update mark prices on positions
	for i := range pe.Positions {
		if entry, ok := prices[pe.Positions[i].Symbol]; ok {
			pe.Positions[i].Mark = entry.Price
		}
	}

	// Check pending orders
	var remaining []PaperOrder
	for _, order := range pe.PendingOrders {
		entry, ok := prices[order.Symbol]
		if !ok {
			remaining = append(remaining, order)
			continue
		}

		triggered := false
		switch order.Type {
		case OrderTypeLimit:
			if order.Side == "BUY" && entry.Ask <= order.Price {
				triggered = true
				order.FilledAt = entry.Ask
			} else if order.Side == "SELL" && entry.Bid >= order.Price {
				triggered = true
				order.FilledAt = entry.Bid
			}
		case OrderTypeStopLimit:
			if order.Side == "BUY" && entry.Ask >= order.Price {
				triggered = true
				order.FilledAt = entry.Ask
			} else if order.Side == "SELL" && entry.Bid <= order.Price {
				triggered = true
				order.FilledAt = entry.Bid
			}
		}

		if triggered {
			order.Status = "FILLED"
			order.FilledTime = time.Now()
			pe.applyFill(order)
			pe.OrderHistory = append(pe.OrderHistory, order)
		} else {
			remaining = append(remaining, order)
		}
	}
	pe.PendingOrders = remaining
}

// applyFill updates positions based on a filled order. Caller must hold lock.
func (pe *PaperEngine) applyFill(order PaperOrder) {
	positionSide := "LONG"
	if order.Side == "SELL" {
		positionSide = "SHORT"
	}

	// Check if we have an opposing position to close
	for i, pos := range pe.Positions {
		if pos.Symbol != order.Symbol {
			continue
		}
		if (pos.Side == "LONG" && order.Side == "SELL") || (pos.Side == "SHORT" && order.Side == "BUY") {
			// Closing position
			pnl := pos.PnL()
			pe.Balance += pnl + pos.Entry*pos.Size // Return capital + PnL
			pe.logPaperTrade(pos, order.FilledAt)
			pe.Positions = append(pe.Positions[:i], pe.Positions[i+1:]...)
			return
		}
	}

	// Open new position
	cost := order.FilledAt * order.Quantity
	if cost > pe.Balance {
		return // Insufficient funds
	}
	pe.Balance -= cost

	pe.Positions = append(pe.Positions, PaperPosition{
		Symbol: order.Symbol,
		Side:   positionSide,
		Size:   order.Quantity,
		Entry:  order.FilledAt,
		Mark:   order.FilledAt,
	})
}

func (pe *PaperEngine) logPaperTrade(pos PaperPosition, exitPrice float64) {
	if pe.DB == nil {
		return
	}

	pnl := pos.PnL()
	pnlPct := pos.PnLPct()

	_ = pe.DB.LogTrade(persistence.TradeRecord{
		Symbol:     pos.Symbol,
		Side:       pos.Side,
		EntryPrice: pos.Entry,
		ExitPrice:  exitPrice,
		Quantity:   pos.Size,
		PnL:       pnl,
		PnLPct:    pnlPct,
		EntryTime:  time.Now().Add(-time.Hour), // approximate
		ExitTime:   time.Now(),
		Exchange:   "paper",
		Paper:      true,
	})
}

// SetBestBidAsk stores the current best bid and ask for market order fills.
func (pe *PaperEngine) SetBestBidAsk(bid, ask float64) {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	pe.bestBid = bid
	pe.bestAsk = ask
}

// UpdateMarkPrice updates the mark price on all positions for the given symbol.
func (pe *PaperEngine) UpdateMarkPrice(symbol string, price float64) {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	for i := range pe.Positions {
		if pe.Positions[i].Symbol == symbol {
			pe.Positions[i].Mark = price
		}
	}
}

// CheckTradePrice checks pending orders against a live trade price and fills if triggered.
func (pe *PaperEngine) CheckTradePrice(symbol string, tradePrice float64) {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	var remaining []PaperOrder
	for _, order := range pe.PendingOrders {
		if order.Symbol != symbol {
			remaining = append(remaining, order)
			continue
		}

		triggered := false
		switch order.Type {
		case OrderTypeLimit:
			if order.Side == "BUY" && tradePrice <= order.Price {
				triggered = true
				order.FilledAt = order.Price
			} else if order.Side == "SELL" && tradePrice >= order.Price {
				triggered = true
				order.FilledAt = order.Price
			}
		case OrderTypeStopLimit:
			if order.Side == "BUY" && tradePrice >= order.Price {
				triggered = true
				order.FilledAt = tradePrice
			} else if order.Side == "SELL" && tradePrice <= order.Price {
				triggered = true
				order.FilledAt = tradePrice
			}
		}

		if triggered {
			order.Status = "FILLED"
			order.FilledTime = time.Now()
			pe.applyFill(order)
			pe.OrderHistory = append(pe.OrderHistory, order)
		} else {
			remaining = append(remaining, order)
		}
	}
	pe.PendingOrders = remaining
}

// TotalPnL returns unrealized PnL across all positions.
func (pe *PaperEngine) TotalPnL() float64 {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	var total float64
	for _, p := range pe.Positions {
		total += p.PnL()
	}
	return total
}

// TotalEquity returns balance + unrealized PnL.
func (pe *PaperEngine) TotalEquity() float64 {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	equity := pe.Balance
	for _, p := range pe.Positions {
		equity += p.Entry*p.Size + p.PnL()
	}
	return equity
}

// GetPositions returns a copy of current positions.
func (pe *PaperEngine) GetPositions() []PaperPosition {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	out := make([]PaperPosition, len(pe.Positions))
	copy(out, pe.Positions)
	return out
}

// PaperPositionsView renders paper positions as a panel.
type PaperPositionsView struct {
	Engine *PaperEngine
	Width  int
	Height int
	Theme  theme.Theme
}

// View renders the paper positions panel.
func (pv PaperPositionsView) View() string {
	if pv.Engine == nil || !pv.Engine.Active {
		return ""
	}

	t := pv.Theme
	w := pv.Width
	if w < 20 {
		w = 60
	}
	innerW := w - 4

	var lines []string
	lines = append(lines, t.Warning.Bold(true).Render(centerPad("PAPER MODE", innerW)))
	lines = append(lines, t.Normal.Render(fmt.Sprintf("  Balance: $%.2f  |  Equity: $%.2f  |  PnL: $%+.2f",
		pv.Engine.Balance, pv.Engine.TotalEquity(), pv.Engine.TotalPnL())))
	lines = append(lines, "")

	positions := pv.Engine.GetPositions()
	if len(positions) == 0 {
		lines = append(lines, t.Dim.Render(centerPad("No paper positions", innerW)))
	} else {
		header := fmt.Sprintf("%-9s %5s %7s %9s %9s %9s", "Symbol", "Side", "Size", "Entry", "Mark", "PnL")
		lines = append(lines, t.TableHeader.Render(truncOrPad(header, innerW)))

		for _, p := range positions {
			pnl := p.PnL()
			pnlStr := fmt.Sprintf("%+.2f", pnl)
			tag := " [PAPER]"

			sym := t.Normal.Render(fmt.Sprintf("%-9s", p.Symbol))
			var sideStyle string
			if p.Side == "LONG" {
				sideStyle = t.PriceUp.Render(fmt.Sprintf(" %5s", p.Side))
			} else {
				sideStyle = t.PriceDown.Render(fmt.Sprintf(" %5s", p.Side))
			}

			rest := t.Normal.Render(fmt.Sprintf(" %7.2f %9.2f %9.2f", p.Size, p.Entry, p.Mark))
			var pnlStyle string
			if pnl >= 0 {
				pnlStyle = t.PriceUp.Render(fmt.Sprintf(" %9s", pnlStr))
			} else {
				pnlStyle = t.PriceDown.Render(fmt.Sprintf(" %9s", pnlStr))
			}
			tagStyle := t.Warning.Render(tag)

			lines = append(lines, truncOrPad(sym+sideStyle+rest+pnlStyle+tagStyle, innerW))
		}
	}

	// Pending orders
	pe := pv.Engine
	pe.mu.RLock()
	pending := pe.PendingOrders
	pe.mu.RUnlock()

	if len(pending) > 0 {
		lines = append(lines, "")
		lines = append(lines, t.Info.Render("  Pending Orders:"))
		for _, o := range pending {
			lines = append(lines, t.Dim.Render(fmt.Sprintf("  %s %s %.4f x%.4f @ %.2f",
				o.Type, o.Side, o.Quantity, o.Quantity, o.Price)))
		}
	}

	content := strings.Join(lines, "\n")
	return t.PanelActive.Width(w - 2).Render(content)
}
