package views

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ashark-ai-05/tradefox/internal/tui/theme"
)

// BacktestState tracks the lifecycle of a backtest run.
type BacktestState int

const (
	BacktestIdle     BacktestState = iota
	BacktestRunning
	BacktestComplete
	BacktestError
)

// BacktestEquityPoint is a point on the equity curve.
type BacktestEquityPoint struct {
	TimestampNs int64
	Equity      float64
	Drawdown    float64
}

// BacktestTradeRecord is a single trade from the backtest.
type BacktestTradeRecord struct {
	Symbol     string
	Side       string
	EntryPrice float64
	ExitPrice  float64
	Quantity   float64
	PnL        float64
	PnLPct     float64
}

// BacktestStats holds computed statistics.
type BacktestStats struct {
	TotalReturn  float64
	SharpeRatio  float64
	MaxDrawdown  float64
	WinRate      float64
	ProfitFactor float64
	TotalTrades  int
}

// BacktestView renders the backtest configuration, progress, and results (Tab 6).
type BacktestView struct {
	Width  int
	Height int
	Theme  theme.Theme

	// Config form fields
	Strategy      string
	Symbol        string
	DateFrom      string
	DateTo        string
	Capital       string
	RiskPct       string
	SelectedField int

	// State
	State    BacktestState
	Progress int
	ErrorMsg string

	// Results
	Stats       BacktestStats
	EquityCurve []BacktestEquityPoint
	Trades      []BacktestTradeRecord
	TradeScroll int

	// Available strategies
	Strategies []string
	StratIdx   int
}

// NewBacktestView creates a backtest view with defaults.
func NewBacktestView(t theme.Theme) BacktestView {
	return BacktestView{
		Theme:      t,
		Strategy:   "scalp_absorption",
		Symbol:     "BTCUSDT",
		DateFrom:   "2024-01-01",
		DateTo:     "2024-12-31",
		Capital:    "10000",
		RiskPct:    "2.0",
		Strategies: []string{"scalp_absorption", "day_fvg", "swing_liquidity"},
	}
}

// SetSize updates the backtest view dimensions.
func (b *BacktestView) SetSize(w, h int) {
	b.Width = w
	b.Height = h
}

// View renders the backtest panel.
func (b BacktestView) View() string {
	t := b.Theme
	w := b.Width
	if w < 40 {
		w = 120
	}
	innerW := w - 4
	h := b.Height
	if h < 10 {
		h = 35
	}

	var sections []string

	// Title
	sections = append(sections, t.Bright.Render(centerPad("Backtest", innerW)))
	sections = append(sections, "")

	// Config form
	sections = append(sections, b.renderConfigForm(t, innerW)...)
	sections = append(sections, "")

	// Run button / progress
	sections = append(sections, b.renderRunState(t, innerW))
	sections = append(sections, "")

	// Results (if complete)
	if b.State == BacktestComplete {
		sections = append(sections, b.renderResults(t, innerW)...)
	} else if b.State == BacktestError {
		sections = append(sections, t.Warning.Render("  Error: "+b.ErrorMsg))
	}

	content := strings.Join(sections, "\n")

	return t.PanelActive.
		Width(w - 2).
		Height(h).
		Render(content)
}

func (b BacktestView) renderConfigForm(t theme.Theme, w int) []string {
	fields := []struct {
		label string
		value string
	}{
		{"Strategy", b.Strategy},
		{"Symbol", b.Symbol},
		{"From", b.DateFrom},
		{"To", b.DateTo},
		{"Capital", b.Capital},
		{"Risk %", b.RiskPct},
	}

	var lines []string
	for i, f := range fields {
		cursor := "  "
		style := t.Normal
		if i == b.SelectedField {
			cursor = "> "
			style = t.Bright
		}
		line := fmt.Sprintf("%s%-12s %s", cursor, f.label+":", f.value)
		lines = append(lines, style.Render(line))
	}
	return lines
}

func (b BacktestView) renderRunState(t theme.Theme, w int) string {
	switch b.State {
	case BacktestIdle:
		return t.Dim.Render("  [Enter to run backtest]")
	case BacktestRunning:
		barW := 30
		filled := b.Progress * barW / 100
		if filled > barW {
			filled = barW
		}
		bar := strings.Repeat("=", filled) + strings.Repeat("-", barW-filled)
		return t.Warning.Render(fmt.Sprintf("  Running... [%s] %d%%", bar, b.Progress))
	case BacktestComplete:
		return t.Bright.Render("  [Complete] Press Enter to run again")
	case BacktestError:
		return t.Warning.Render("  [Error] Press Enter to retry")
	}
	return ""
}

func (b BacktestView) renderResults(t theme.Theme, w int) []string {
	var lines []string

	// Stats table and equity curve side by side
	statsW := w / 2
	if statsW < 30 {
		statsW = 30
	}

	// Stats
	lines = append(lines, t.Bright.Render("  --- Stats ---"))
	lines = append(lines, t.Normal.Render(fmt.Sprintf("  Return:       %.2f%%", b.Stats.TotalReturn*100)))
	lines = append(lines, t.Normal.Render(fmt.Sprintf("  Sharpe:       %.2f", b.Stats.SharpeRatio)))
	lines = append(lines, t.Normal.Render(fmt.Sprintf("  Max DD:       %.2f%%", b.Stats.MaxDrawdown*100)))
	lines = append(lines, t.Normal.Render(fmt.Sprintf("  Win Rate:     %.1f%%", b.Stats.WinRate*100)))
	lines = append(lines, t.Normal.Render(fmt.Sprintf("  Profit Fac:   %.2f", b.Stats.ProfitFactor)))
	lines = append(lines, t.Normal.Render(fmt.Sprintf("  Trades:       %d", b.Stats.TotalTrades)))
	lines = append(lines, "")

	// Equity curve (braille sparkline)
	if len(b.EquityCurve) > 0 {
		lines = append(lines, t.Bright.Render("  --- Equity Curve ---"))
		sparkline := b.renderSparkline(w-6, 6)
		for _, sl := range sparkline {
			lines = append(lines, "  "+sl)
		}
		lines = append(lines, "")
	}

	// Trade list
	if len(b.Trades) > 0 {
		lines = append(lines, t.Bright.Render("  --- Trades ---"))
		header := fmt.Sprintf("  %-4s %-6s %12s %12s %10s", "#", "Side", "Entry", "Exit", "PnL%")
		lines = append(lines, t.Dim.Render(header))

		maxVisible := 10
		start := b.TradeScroll
		if start > len(b.Trades)-maxVisible {
			start = len(b.Trades) - maxVisible
		}
		if start < 0 {
			start = 0
		}
		end := start + maxVisible
		if end > len(b.Trades) {
			end = len(b.Trades)
		}

		for i := start; i < end; i++ {
			tr := b.Trades[i]
			pnlStyle := t.Normal
			if tr.PnL > 0 {
				pnlStyle = lipgloss.NewStyle().Foreground(t.Colors.PriceUp)
			} else if tr.PnL < 0 {
				pnlStyle = lipgloss.NewStyle().Foreground(t.Colors.PriceDown)
			}
			line := fmt.Sprintf("  %-4d %-6s %12.2f %12.2f ", i+1, tr.Side, tr.EntryPrice, tr.ExitPrice)
			pnlStr := fmt.Sprintf("%+.2f%%", tr.PnLPct)
			lines = append(lines, t.Normal.Render(line)+pnlStyle.Render(pnlStr))
		}

		if len(b.Trades) > maxVisible {
			lines = append(lines, t.Dim.Render(fmt.Sprintf("  [j/k to scroll, %d/%d]", start+1, len(b.Trades))))
		}
	}

	return lines
}

func (b BacktestView) renderSparkline(width, height int) []string {
	if len(b.EquityCurve) == 0 || width < 5 || height < 2 {
		return nil
	}

	// Sample equity values to fit width
	values := make([]float64, width)
	for i := range values {
		idx := i * len(b.EquityCurve) / width
		if idx >= len(b.EquityCurve) {
			idx = len(b.EquityCurve) - 1
		}
		values[i] = b.EquityCurve[idx].Equity
	}

	// Find min/max
	minV, maxV := values[0], values[0]
	for _, v := range values {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	rangeV := maxV - minV
	if rangeV == 0 {
		rangeV = 1
	}

	// Build character grid using braille-like characters
	blocks := []string{" ", ".", ":", "^", "*", "#"}
	rows := height
	grid := make([][]byte, rows)
	for r := range grid {
		grid[r] = make([]byte, width)
		for c := range grid[r] {
			grid[r][c] = ' '
		}
	}

	for col, v := range values {
		normalized := (v - minV) / rangeV
		row := int(math.Round(float64(rows-1) * (1.0 - normalized)))
		if row < 0 {
			row = 0
		}
		if row >= rows {
			row = rows - 1
		}
		blockIdx := int(normalized * float64(len(blocks)-1))
		if blockIdx >= len(blocks) {
			blockIdx = len(blocks) - 1
		}
		grid[row][col] = blocks[blockIdx][0]
	}

	var lines []string
	for _, row := range grid {
		lines = append(lines, string(row))
	}
	return lines
}

// CycleStrategy moves to the next strategy in the list.
func (b *BacktestView) CycleStrategy() {
	if len(b.Strategies) == 0 {
		return
	}
	b.StratIdx = (b.StratIdx + 1) % len(b.Strategies)
	b.Strategy = b.Strategies[b.StratIdx]
}

// TypeChar adds a character to the currently selected field.
func (b *BacktestView) TypeChar(ch string) {
	switch b.SelectedField {
	case 1:
		b.Symbol += ch
	case 2:
		b.DateFrom += ch
	case 3:
		b.DateTo += ch
	case 4:
		b.Capital += ch
	case 5:
		b.RiskPct += ch
	}
}

// Backspace removes a character from the selected field.
func (b *BacktestView) Backspace() {
	switch b.SelectedField {
	case 1:
		if len(b.Symbol) > 0 {
			b.Symbol = b.Symbol[:len(b.Symbol)-1]
		}
	case 2:
		if len(b.DateFrom) > 0 {
			b.DateFrom = b.DateFrom[:len(b.DateFrom)-1]
		}
	case 3:
		if len(b.DateTo) > 0 {
			b.DateTo = b.DateTo[:len(b.DateTo)-1]
		}
	case 4:
		if len(b.Capital) > 0 {
			b.Capital = b.Capital[:len(b.Capital)-1]
		}
	case 5:
		if len(b.RiskPct) > 0 {
			b.RiskPct = b.RiskPct[:len(b.RiskPct)-1]
		}
	}
}
