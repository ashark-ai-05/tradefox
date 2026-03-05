package components

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/ashark-ai-05/tradefox/internal/tui/theme"
)

// HeaderData holds the data displayed in the header.
type HeaderData struct {
	Symbol     string
	Price      float64
	Change24   float64
	Spread     float64
	Connected  bool
	Exchange   string
}

// Header is the top bar component.
type Header struct {
	Data  HeaderData
	Width int
	Theme theme.Theme
}

// NewHeader creates a new header.
func NewHeader(t theme.Theme) Header {
	return Header{
		Theme: t,
		Data: HeaderData{
			Symbol:    "BTCUSDT",
			Price:     67432.50,
			Change24:  2.34,
			Spread:    1.00,
			Connected: true,
			Exchange:  "Binance",
		},
	}
}

// View renders the header bar.
func (h Header) View() string {
	t := h.Theme

	sym := t.Header.Render(fmt.Sprintf(" %s ", h.Data.Symbol))

	priceStr := fmt.Sprintf(" $%.2f ", h.Data.Price)
	price := t.Header.Foreground(t.Colors.FgBright).Render(priceStr)

	changeStr := fmt.Sprintf(" %.2f%% ", h.Data.Change24)
	var change string
	if h.Data.Change24 >= 0 {
		change = t.Header.Foreground(t.Colors.PriceUp).Render("+" + changeStr)
	} else {
		change = t.Header.Foreground(t.Colors.PriceDown).Render(changeStr)
	}

	spread := t.Header.Foreground(t.Colors.FgDim).Render(fmt.Sprintf(" Spread: %.2f ", h.Data.Spread))

	var connStatus string
	if h.Data.Connected {
		connStatus = t.Header.Foreground(t.Colors.PriceUp).Render(" ● Connected ")
	} else {
		connStatus = t.Header.Foreground(t.Colors.PriceDown).Render(" ○ Disconnected ")
	}

	exchange := t.Header.Foreground(t.Colors.Info).Render(fmt.Sprintf(" %s ", h.Data.Exchange))

	left := lipgloss.JoinHorizontal(lipgloss.Center, sym, price, change, spread)
	right := lipgloss.JoinHorizontal(lipgloss.Center, exchange, connStatus)

	w := h.Width
	if w < 1 {
		w = 120
	}

	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 1
	}
	filler := t.Header.Render(fmt.Sprintf("%*s", gap, ""))

	bar := lipgloss.JoinHorizontal(lipgloss.Center, left, filler, right)
	return lipgloss.NewStyle().
		Width(w).
		Background(t.Colors.HeaderBg).
		Render(bar)
}
