package views

import (
	"fmt"
	"strings"

	"github.com/ashark-ai-05/tradefox/internal/tui/mock"
	"github.com/ashark-ai-05/tradefox/internal/tui/theme"
)

// PositionsView displays open positions.
type PositionsView struct {
	Entries []mock.PositionEntry
	Width   int
	Height  int
	Theme   theme.Theme
}

// NewPositionsView creates a positions view with mock data.
func NewPositionsView(t theme.Theme) PositionsView {
	return PositionsView{
		Entries: mock.Positions(),
		Theme:   t,
	}
}

// SetSize updates the positions view dimensions.
func (p *PositionsView) SetSize(w, h int) {
	p.Width = w
	p.Height = h
}

// View renders the positions panel.
func (p PositionsView) View() string {
	t := p.Theme
	w := p.Width
	if w < 20 {
		w = 40
	}
	innerW := w - 4

	title := t.TableHeader.Render(centerPad("Positions", innerW))

	header := fmt.Sprintf("%-9s %5s %7s %9s %9s %9s %7s",
		"Symbol", "Side", "Size", "Entry", "Mark", "PnL", "PnL%")
	headerLine := t.TableHeader.Render(truncOrPad(header, innerW))

	var rows []string
	rows = append(rows, title)
	rows = append(rows, headerLine)

	totalPnL := 0.0
	for _, e := range p.Entries {
		pnl := e.PnL()
		pnlPct := e.PnLPct()
		totalPnL += pnl

		sideStyle := t.PriceUp
		if e.Side == "SHORT" {
			sideStyle = t.PriceDown
		}

		pnlStyle := t.PriceUp
		if pnl < 0 {
			pnlStyle = t.PriceDown
		}

		sym := t.Normal.Render(fmt.Sprintf("%-9s", e.Symbol))
		side := sideStyle.Render(fmt.Sprintf(" %5s", e.Side))

		var sizeFmt string
		if e.Size >= 100 {
			sizeFmt = fmt.Sprintf("%.0f", e.Size)
		} else {
			sizeFmt = fmt.Sprintf("%.2f", e.Size)
		}

		var entryFmt, markFmt string
		if e.Entry >= 100 {
			entryFmt = fmt.Sprintf("%.2f", e.Entry)
			markFmt = fmt.Sprintf("%.2f", e.Mark)
		} else {
			entryFmt = fmt.Sprintf("%.4f", e.Entry)
			markFmt = fmt.Sprintf("%.4f", e.Mark)
		}

		rest := t.Normal.Render(fmt.Sprintf(" %7s %9s %9s", sizeFmt, entryFmt, markFmt))
		pnlStr := pnlStyle.Render(fmt.Sprintf(" %9.2f %+6.2f%%", pnl, pnlPct))

		rows = append(rows, truncOrPad(sym+side+rest+pnlStr, innerW))
	}

	// Summary
	rows = append(rows, t.Dim.Render(strings.Repeat("─", innerW)))
	totalStyle := t.PriceUp
	if totalPnL < 0 {
		totalStyle = t.PriceDown
	}
	summary := totalStyle.Render(fmt.Sprintf("  Total PnL: $%.2f", totalPnL))
	rows = append(rows, summary)

	content := strings.Join(rows, "\n")

	h := p.Height
	if h < 1 {
		h = len(p.Entries) + 6
	}

	return t.PanelActive.
		Width(w - 2).
		Height(h).
		Render(content)
}
