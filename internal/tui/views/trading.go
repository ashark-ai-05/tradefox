package views

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/ashark-ai-05/tradefox/internal/tui/theme"
)

// TradingView is the composite main trading view (Tab 1).
// It joins the order book, watchlist, and positions side by side.
type TradingView struct {
	OrderBook OrderBookView
	Watchlist WatchlistView
	Positions PositionsView
	Width     int
	Height    int
	Theme     theme.Theme
}

// NewTradingView creates the trading view with all sub-components.
func NewTradingView(t theme.Theme) TradingView {
	return TradingView{
		OrderBook: NewOrderBookView(t),
		Watchlist: NewWatchlistView(t),
		Positions: NewPositionsView(t),
		Theme:     t,
	}
}

// View renders the three-column trading layout.
func (tv TradingView) View() string {
	w := tv.Width
	if w < 60 {
		w = 120
	}
	h := tv.Height
	if h < 10 {
		h = 35
	}

	leftW := w * 30 / 100
	centerW := w * 40 / 100
	rightW := w - leftW - centerW

	tv.OrderBook.Width = leftW
	tv.OrderBook.Height = h
	tv.Watchlist.Width = centerW
	tv.Watchlist.Height = h
	tv.Positions.Width = rightW
	tv.Positions.Height = h

	left := tv.OrderBook.View()
	center := tv.Watchlist.View()
	right := tv.Positions.View()

	return lipgloss.JoinHorizontal(lipgloss.Top, left, center, right)
}
