package views

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/ashark-ai-05/tradefox/internal/tui/theme"
)

// TradingView is the composite main trading view (Tab 1).
// Enhanced layout: chart + liquidation heatmap + order book on top,
// signals + trades + positions on bottom.
type TradingView struct {
	Chart       ChartView
	Liquidation LiquidationView
	OrderBook   OrderBookView
	Signals     SignalsView
	Trades      TradesView
	Watchlist   WatchlistView
	Positions   PositionsView
	Width       int
	Height      int
	Theme       theme.Theme
}

// NewTradingView creates the trading view with all sub-components.
func NewTradingView(t theme.Theme) TradingView {
	return TradingView{
		Chart:       NewChartView(t),
		Liquidation: NewLiquidationView(t),
		OrderBook:   NewOrderBookView(t),
		Signals:     NewSignalsView(t),
		Trades:      NewTradesView(t),
		Watchlist:   NewWatchlistView(t),
		Positions:   NewPositionsView(t),
		Theme:       t,
	}
}

// View renders the enhanced trading layout.
func (tv TradingView) View() string {
	w := tv.Width
	if w < 60 {
		w = 120
	}
	h := tv.Height
	if h < 10 {
		h = 35
	}

	// Top section: 60% height, Bottom section: 40% height
	topH := h * 60 / 100
	botH := h - topH

	// Top-left: Chart (60%) + Liquidation heatmap (8 chars wide)
	// Top-right: Order book (25%)
	liqW := 10
	orderBookW := w * 25 / 100
	chartW := w - liqW - orderBookW

	tv.Chart.Width = chartW
	tv.Chart.Height = topH
	tv.Liquidation.Width = liqW
	tv.Liquidation.Height = topH
	tv.OrderBook.Width = orderBookW
	tv.OrderBook.Height = topH

	chartView := tv.Chart.View()
	liqView := tv.Liquidation.View()
	orderView := tv.OrderBook.View()

	var topRow string
	if tv.Liquidation.Visible {
		topRow = lipgloss.JoinHorizontal(lipgloss.Top, chartView, liqView, orderView)
	} else {
		tv.Chart.Width = chartW + liqW
		chartView = tv.Chart.View()
		topRow = lipgloss.JoinHorizontal(lipgloss.Top, chartView, orderView)
	}

	// Bottom-left: Signal dashboard (40%)
	// Bottom-center: Recent trades feed (30%)
	// Bottom-right: Positions + order entry (30%)
	sigW := w * 40 / 100
	tradesW := w * 30 / 100
	posW := w - sigW - tradesW

	tv.Signals.Width = sigW
	tv.Signals.Height = botH
	tv.Trades.Width = tradesW
	tv.Trades.Height = botH
	tv.Positions.Width = posW
	tv.Positions.Height = botH

	sigView := tv.Signals.View()
	tradesView := tv.Trades.View()
	posView := tv.Positions.View()

	botRow := lipgloss.JoinHorizontal(lipgloss.Top, sigView, tradesView, posView)

	return lipgloss.JoinVertical(lipgloss.Left, topRow, botRow)
}
