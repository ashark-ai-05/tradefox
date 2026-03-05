package components

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/ashark-ai-05/tradefox/internal/tui/theme"
)

// StatusBarData holds data for the bottom status bar.
type StatusBarData struct {
	Exchange    string
	Positions   int
	DailyPnL    float64
	RiskStatus  string
}

// StatusBar is the bottom bar component.
type StatusBar struct {
	Data  StatusBarData
	Width int
	Theme theme.Theme
}

// NewStatusBar creates a new status bar.
func NewStatusBar(t theme.Theme) StatusBar {
	return StatusBar{
		Theme: t,
		Data: StatusBarData{
			Exchange:   "Binance Futures",
			Positions:  4,
			DailyPnL:   1247.35,
			RiskStatus: "Normal",
		},
	}
}

// View renders the status bar.
func (s StatusBar) View() string {
	t := s.Theme

	exchange := t.StatusBar.Foreground(t.Colors.Info).Render(fmt.Sprintf(" %s ", s.Data.Exchange))

	posStr := fmt.Sprintf(" Positions: %d ", s.Data.Positions)
	positions := t.StatusBar.Render(posStr)

	pnlStr := fmt.Sprintf(" PnL: $%.2f ", s.Data.DailyPnL)
	var pnl string
	if s.Data.DailyPnL >= 0 {
		pnl = t.StatusBar.Foreground(t.Colors.PriceUp).Render(pnlStr)
	} else {
		pnl = t.StatusBar.Foreground(t.Colors.PriceDown).Render(pnlStr)
	}

	risk := t.StatusBar.Foreground(t.Colors.PriceUp).Render(fmt.Sprintf(" Risk: %s ", s.Data.RiskStatus))

	clock := t.StatusBar.Render(fmt.Sprintf(" %s ", time.Now().Format("15:04:05")))

	keys := t.StatusBar.Foreground(t.Colors.FgDim).Render(
		" [Tab] Switch | [b] Buy | [s] Sell | [?] Help | [q] Quit ")

	left := lipgloss.JoinHorizontal(lipgloss.Center, exchange, positions, pnl, risk)
	right := lipgloss.JoinHorizontal(lipgloss.Center, keys, clock)

	w := s.Width
	if w < 1 {
		w = 120
	}

	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 1
	}
	filler := t.StatusBar.Render(fmt.Sprintf("%*s", gap, ""))

	bar := lipgloss.JoinHorizontal(lipgloss.Center, left, filler, right)
	return lipgloss.NewStyle().
		Width(w).
		Background(t.Colors.StatusBarBg).
		Render(bar)
}
