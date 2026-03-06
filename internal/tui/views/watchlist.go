package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ashark-ai-05/tradefox/internal/tui/mock"
	"github.com/ashark-ai-05/tradefox/internal/tui/theme"
)

// WatchlistView shows a table of instruments.
type WatchlistView struct {
	Entries  []mock.WatchlistEntry
	Cursor   int
	Width    int
	Height   int
	Theme    theme.Theme
}

// NewWatchlistView creates a new watchlist with mock data.
func NewWatchlistView(t theme.Theme) WatchlistView {
	return WatchlistView{
		Entries: mock.Watchlist(),
		Theme:   t,
	}
}

// WatchlistTickerData matches tui.WatchlistTickerData to avoid import cycle.
type WatchlistTickerData struct {
	Symbol   string
	Price    float64
	Change24 float64
	Volume   float64
}

// UpdateLivePrices updates watchlist entries with live ticker data.
func (w *WatchlistView) UpdateLivePrices(tickers []WatchlistTickerData) {
	lookup := make(map[string]WatchlistTickerData, len(tickers))
	for _, t := range tickers {
		lookup[t.Symbol] = t
	}
	for i := range w.Entries {
		if t, ok := lookup[w.Entries[i].Symbol]; ok {
			w.Entries[i].Price = t.Price
			w.Entries[i].Change24 = t.Change24
			w.Entries[i].Volume = t.Volume
			// Update bid/ask estimates based on price.
			spread := w.Entries[i].Spread()
			if spread <= 0 {
				spread = t.Price * 0.0001
			}
			w.Entries[i].Bid = t.Price - spread/2
			w.Entries[i].Ask = t.Price + spread/2
		}
	}
}

// SelectedEntry returns the currently selected entry.
func (w WatchlistView) SelectedEntry() mock.WatchlistEntry {
	if w.Cursor >= 0 && w.Cursor < len(w.Entries) {
		return w.Entries[w.Cursor]
	}
	return mock.WatchlistEntry{}
}

// SetSize updates the watchlist dimensions.
func (wv *WatchlistView) SetSize(width, height int) {
	wv.Width = width
	wv.Height = height
}

// View renders the watchlist table.
func (w WatchlistView) View() string {
	t := w.Theme
	width := w.Width
	if width < 30 {
		width = 50
	}
	innerW := width - 4

	title := t.TableHeader.Render(centerPad("Watchlist", innerW))

	header := fmt.Sprintf("%-10s %10s %7s %9s %10s %10s %7s",
		"Symbol", "Price", "24h%", "Volume", "Bid", "Ask", "Spread")
	headerLine := t.TableHeader.Render(truncOrPad(header, innerW))

	h := w.Height
	if h < 1 {
		h = 25
	}
	maxRows := h - 4
	if maxRows < 1 {
		maxRows = 1
	}

	var rows []string
	rows = append(rows, title)
	rows = append(rows, headerLine)

	for i, e := range w.Entries {
		if i >= maxRows {
			break
		}

		changeStr := fmt.Sprintf("%+.2f%%", e.Change24)
		volStr := mock.FormatVolume(e.Volume)

		var priceFmt string
		if e.Price >= 100 {
			priceFmt = fmt.Sprintf("%.2f", e.Price)
		} else if e.Price >= 1 {
			priceFmt = fmt.Sprintf("%.4f", e.Price)
		} else {
			priceFmt = fmt.Sprintf("%.4f", e.Price)
		}

		var bidFmt, askFmt, spreadFmt string
		if e.Bid >= 100 {
			bidFmt = fmt.Sprintf("%.2f", e.Bid)
			askFmt = fmt.Sprintf("%.2f", e.Ask)
			spreadFmt = fmt.Sprintf("%.2f", e.Spread())
		} else {
			bidFmt = fmt.Sprintf("%.4f", e.Bid)
			askFmt = fmt.Sprintf("%.4f", e.Ask)
			spreadFmt = fmt.Sprintf("%.4f", e.Spread())
		}

		line := fmt.Sprintf("%-10s %10s %7s %9s %10s %10s %7s",
			e.Symbol, priceFmt, changeStr, volStr, bidFmt, askFmt, spreadFmt)

		if i == w.Cursor {
			rows = append(rows, t.TableRowSel.Render(truncOrPad(line, innerW)))
		} else {
			var style lipgloss.Style
			if e.Change24 >= 0 {
				style = t.PriceUp
			} else {
				style = t.PriceDown
			}
			// Only color the change; rest is normal
			symPart := t.Normal.Render(fmt.Sprintf("%-10s %10s", e.Symbol, priceFmt))
			changePart := style.Render(fmt.Sprintf(" %7s", changeStr))
			restPart := t.Normal.Render(fmt.Sprintf(" %9s %10s %10s %7s", volStr, bidFmt, askFmt, spreadFmt))
			combined := symPart + changePart + restPart
			rows = append(rows, truncOrPad(combined, innerW))
		}
	}

	content := strings.Join(rows, "\n")

	return t.PanelActive.
		Width(width - 2).
		Height(h).
		Render(content)
}
