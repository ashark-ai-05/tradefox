package tui

import (
	"github.com/ashark-ai-05/tradefox/internal/tui/live"
)

// OrderBookUpdateMsg carries live order book data into the TUI.
type OrderBookUpdateMsg struct {
	Bids []live.OrderBookLevel
	Asks []live.OrderBookLevel
}

// TradeUpdateMsg carries a live trade event into the TUI.
type TradeUpdateMsg struct {
	Trade live.TradeEvent
}

// TickerUpdateMsg carries a live ticker/mark price update into the TUI.
type TickerUpdateMsg struct {
	Price       float64
	Change24h   float64
	FundingRate float64
	Bid         float64
	Ask         float64
}

// CandleUpdateMsg carries a live candle update into the TUI.
type CandleUpdateMsg struct {
	Candle live.Candle
}

// ConnectionStatusMsg carries a connection status change into the TUI.
type ConnectionStatusMsg struct {
	Status    live.ConnectionStatus
	Connected bool
}

// WatchlistTickerData holds data for a single watchlist ticker from REST API.
type WatchlistTickerData struct {
	Symbol   string
	Price    float64
	Change24 float64
	Volume   float64
}

// WatchlistUpdateMsg carries live watchlist prices into the TUI.
type WatchlistUpdateMsg struct {
	Tickers []WatchlistTickerData
}

// BacktestProgressMsg carries backtest progress into the TUI.
type BacktestProgressMsg struct {
	BacktestID string
	Pct        int
	Status     string
	Message    string
}

// BacktestResultMsg carries completed backtest results into the TUI.
type BacktestResultMsg struct {
	BacktestID   string
	TotalReturn  float64
	SharpeRatio  float64
	MaxDrawdown  float64
	WinRate      float64
	ProfitFactor float64
	TotalTrades  int
	EquityCurve  []BacktestEquityPointMsg
	Trades       []BacktestTradeMsg
}

// BacktestEquityPointMsg is an equity curve data point.
type BacktestEquityPointMsg struct {
	TimestampNs int64
	Equity      float64
	Drawdown    float64
}

// BacktestTradeMsg is a trade record from the backtest.
type BacktestTradeMsg struct {
	Symbol     string
	Side       string
	EntryPrice float64
	ExitPrice  float64
	Quantity   float64
	PnL        float64
	PnLPct     float64
}
