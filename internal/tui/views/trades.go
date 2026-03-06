package views

import (
	"fmt"
	"strings"

	"github.com/ashark-ai-05/tradefox/internal/tui/mock"
	"github.com/ashark-ai-05/tradefox/internal/tui/theme"
)

// TradesView displays a scrolling recent trades feed.
type TradesView struct {
	Trades []mock.Trade
	Width  int
	Height int
	Theme  theme.Theme
}

// NewTradesView creates a new trades feed.
func NewTradesView(t theme.Theme) TradesView {
	return TradesView{
		Trades: mock.GenerateMockTrades(50),
		Theme:  t,
	}
}

// View renders the recent trades feed.
func (tv TradesView) View() string {
	t := tv.Theme
	w := tv.Width
	if w < 20 {
		w = 35
	}
	h := tv.Height
	if h < 5 {
		h = 15
	}
	innerW := w - 4

	title := t.TableHeader.Render(centerPad("Recent Trades", innerW))

	header := fmt.Sprintf(" %-8s %10s %10s %4s", "Time", "Price", "Size", "Side")
	headerLine := t.TableHeader.Render(truncOrPad(header, innerW))

	var lines []string
	lines = append(lines, title)
	lines = append(lines, headerLine)

	maxRows := h - 4
	if maxRows < 1 {
		maxRows = 1
	}

	for i, trade := range tv.Trades {
		if i >= maxRows {
			break
		}

		minutes := trade.Time / 60
		seconds := trade.Time % 60
		timeStr := fmt.Sprintf("%02d:%02d:%02d", minutes/60, minutes%60, seconds)

		var priceFmt string
		if trade.Price >= 100 {
			priceFmt = fmt.Sprintf("%.2f", trade.Price)
		} else {
			priceFmt = fmt.Sprintf("%.4f", trade.Price)
		}

		var sizeFmt string
		if trade.Size >= 1.0 {
			sizeFmt = fmt.Sprintf("%.3f", trade.Size)
		} else {
			sizeFmt = fmt.Sprintf("%.4f", trade.Size)
		}

		line := fmt.Sprintf(" %-8s %10s %10s %4s", timeStr, priceFmt, sizeFmt, trade.Side)

		isLarge := trade.Size >= 1.0
		style := t.PriceUp
		if trade.Side == "SELL" {
			style = t.PriceDown
		}
		if isLarge {
			style = style.Bold(true)
		}

		lines = append(lines, style.Render(truncOrPad(line, innerW)))
	}

	content := strings.Join(lines, "\n")

	return t.PanelInactive.
		Width(w - 2).
		Height(h).
		Render(content)
}
