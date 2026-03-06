package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/persistence"
	"github.com/ashark-ai-05/tradefox/internal/tui/theme"
)

// JournalView displays the trade journal (Tab 5).
type JournalView struct {
	Trades  []persistence.TradeRecord
	Stats   persistence.PerformanceStats
	Equity  []float64
	Cursor  int
	Width   int
	Height  int
	Theme   theme.Theme

	// Filters
	FilterSymbol string
	FilterSide   string
	FilterSetup  string
	FilterDays   int

	// Editing state
	AddingTrade  bool
	EditingNotes bool
}

// NewJournalView creates a journal view.
func NewJournalView(t theme.Theme) JournalView {
	return JournalView{
		Theme:      t,
		FilterDays: 30,
	}
}

// SetSize updates the journal view dimensions.
func (j *JournalView) SetSize(w, h int) {
	j.Width = w
	j.Height = h
}

// View renders the journal view.
func (j JournalView) View() string {
	t := j.Theme
	w := j.Width
	if w < 40 {
		w = 120
	}
	innerW := w - 4
	h := j.Height
	if h < 10 {
		h = 35
	}

	var sections []string

	// Title
	sections = append(sections, t.TableHeader.Render(centerPad("Trade Journal", innerW)))

	// Stats summary bar
	statsLine := j.renderStats(t, innerW)
	sections = append(sections, statsLine)

	// Equity sparkline
	if len(j.Equity) > 0 {
		spark := j.renderSparkline(t, innerW)
		sections = append(sections, spark)
	}

	// Table header
	header := fmt.Sprintf("%-10s %-9s %5s %10s %10s %9s %5s %-10s %-12s",
		"Date", "Symbol", "Side", "Entry", "Exit", "PnL", "R", "Setup", "Notes")
	sections = append(sections, t.TableHeader.Render(truncOrPad(header, innerW)))

	// Trade rows
	maxRows := h - len(sections) - 3
	if maxRows < 1 {
		maxRows = 1
	}

	for i, tr := range j.Trades {
		if i >= maxRows {
			break
		}

		dateStr := tr.ExitTime.Format("01/02 15:04")
		var entryFmt, exitFmt string
		if tr.EntryPrice >= 100 {
			entryFmt = fmt.Sprintf("%.2f", tr.EntryPrice)
			exitFmt = fmt.Sprintf("%.2f", tr.ExitPrice)
		} else {
			entryFmt = fmt.Sprintf("%.4f", tr.EntryPrice)
			exitFmt = fmt.Sprintf("%.4f", tr.ExitPrice)
		}

		pnlStr := fmt.Sprintf("%+.2f", tr.PnL)
		rStr := fmt.Sprintf("%+.1f", tr.RMultiple)
		notes := tr.Notes
		if len(notes) > 12 {
			notes = notes[:11] + "~"
		}

		paperTag := ""
		if tr.Paper {
			paperTag = "[P]"
		}

		line := fmt.Sprintf("%-10s %-6s%3s %5s %10s %10s %9s %5s %-10s %-12s",
			dateStr, tr.Symbol, paperTag, tr.Side, entryFmt, exitFmt, pnlStr, rStr, tr.SetupType, notes)

		if i == j.Cursor {
			sections = append(sections, t.TableRowSel.Render(truncOrPad(line, innerW)))
		} else {
			// Color PnL
			sym := t.Normal.Render(fmt.Sprintf("%-10s %-6s%3s", dateStr, tr.Symbol, paperTag))
			var sideStyle string
			if tr.Side == "LONG" {
				sideStyle = t.PriceUp.Render(fmt.Sprintf(" %5s", tr.Side))
			} else {
				sideStyle = t.PriceDown.Render(fmt.Sprintf(" %5s", tr.Side))
			}
			prices := t.Normal.Render(fmt.Sprintf(" %10s %10s", entryFmt, exitFmt))
			var pnlStyle string
			if tr.PnL >= 0 {
				pnlStyle = t.PriceUp.Render(fmt.Sprintf(" %9s %5s", pnlStr, rStr))
			} else {
				pnlStyle = t.PriceDown.Render(fmt.Sprintf(" %9s %5s", pnlStr, rStr))
			}
			rest := t.Dim.Render(fmt.Sprintf(" %-10s %-12s", tr.SetupType, notes))
			sections = append(sections, truncOrPad(sym+sideStyle+prices+pnlStyle+rest, innerW))
		}
	}

	if len(j.Trades) == 0 {
		sections = append(sections, t.Dim.Render(centerPad("No trades recorded. Press 'n' to add a trade.", innerW)))
	}

	// Footer
	sections = append(sections, t.Dim.Render(fmt.Sprintf("  [n] New Trade  [e] Edit Notes  [j/k] Navigate  Showing last %d days", j.FilterDays)))

	content := strings.Join(sections, "\n")
	return t.PanelActive.
		Width(w - 2).
		Height(h).
		Render(content)
}

func (j JournalView) renderStats(t theme.Theme, w int) string {
	s := j.Stats
	winRate := fmt.Sprintf("Win: %.0f%%", s.WinRate)
	avgR := fmt.Sprintf("Avg R: %.1f", s.Expectancy)
	pf := fmt.Sprintf("PF: %.2f", s.ProfitFactor)
	totalPnL := fmt.Sprintf("PnL: $%.2f", s.TotalPnL)
	maxDD := fmt.Sprintf("DD: $%.2f", s.MaxDrawdown)
	trades := fmt.Sprintf("Trades: %d", s.TotalTrades)

	var pnlStyled string
	if s.TotalPnL >= 0 {
		pnlStyled = t.PriceUp.Render(totalPnL)
	} else {
		pnlStyled = t.PriceDown.Render(totalPnL)
	}

	parts := []string{
		t.Info.Render(winRate),
		t.Normal.Render(avgR),
		t.Normal.Render(pf),
		pnlStyled,
		t.PriceDown.Render(maxDD),
		t.Dim.Render(trades),
	}

	return "  " + strings.Join(parts, "  |  ")
}

func (j JournalView) renderSparkline(t theme.Theme, w int) string {
	blocks := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	data := j.Equity

	// Scale to available width
	displayW := w - 4
	if displayW < 10 {
		displayW = 10
	}

	// Resample if needed
	if len(data) > displayW {
		step := float64(len(data)) / float64(displayW)
		resampled := make([]float64, displayW)
		for i := range resampled {
			idx := int(float64(i) * step)
			if idx >= len(data) {
				idx = len(data) - 1
			}
			resampled[i] = data[idx]
		}
		data = resampled
	}

	// Find min/max
	minVal, maxVal := data[0], data[0]
	for _, v := range data {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	rng := maxVal - minVal
	if rng == 0 {
		rng = 1
	}

	var spark strings.Builder
	spark.WriteString("  ")
	for _, v := range data {
		normalized := (v - minVal) / rng
		idx := int(normalized * float64(len(blocks)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(blocks) {
			idx = len(blocks) - 1
		}
		spark.WriteRune(blocks[idx])
	}

	// Color based on final equity
	label := fmt.Sprintf(" Equity (%dd)", j.FilterDays)
	if len(j.Equity) > 0 && j.Equity[len(j.Equity)-1] >= 0 {
		return t.PriceUp.Render(spark.String()) + t.Dim.Render(label)
	}
	return t.PriceDown.Render(spark.String()) + t.Dim.Render(label)
}

// RefreshData loads trades and stats from the database.
func (j *JournalView) RefreshData(db *persistence.DB) {
	if db == nil {
		return
	}

	now := time.Now()
	from := now.AddDate(0, 0, -j.FilterDays)

	trades, err := db.GetTrades(from, now)
	if err == nil {
		j.Trades = trades
	}

	stats, err := db.GetPerformanceStats(j.FilterDays)
	if err == nil {
		j.Stats = stats
	}

	equity, err := db.GetDailyEquityCurve(j.FilterDays)
	if err == nil {
		j.Equity = equity
	}
}
