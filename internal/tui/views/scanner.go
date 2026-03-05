package views

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ashark-ai-05/tradefox/internal/tui/mock"
	"github.com/ashark-ai-05/tradefox/internal/tui/theme"
)

// ScannerView shows the EAX scanner table.
type ScannerView struct {
	Entries  []mock.ScannerEntry
	Cursor   int
	SortCol  int
	SortAsc  bool
	Filter   string
	Filtering bool
	Width    int
	Height   int
	Theme    theme.Theme
}

// NewScannerView creates a scanner with mock data.
func NewScannerView(t theme.Theme) ScannerView {
	return ScannerView{
		Entries: mock.Scanner(),
		SortCol: 2, // sort by change% by default
		Theme:   t,
	}
}

// SelectedSymbol returns the symbol at the cursor.
func (s ScannerView) SelectedSymbol() string {
	filtered := s.FilteredEntries()
	if s.Cursor >= 0 && s.Cursor < len(filtered) {
		return filtered[s.Cursor].Symbol
	}
	return ""
}

// FilteredEntries returns entries matching the current filter.
func (s ScannerView) FilteredEntries() []mock.ScannerEntry {
	if s.Filter == "" {
		return s.Entries
	}
	filter := strings.ToUpper(s.Filter)
	var result []mock.ScannerEntry
	for _, e := range s.Entries {
		if strings.Contains(strings.ToUpper(e.Symbol), filter) {
			result = append(result, e)
		}
	}
	return result
}

// SortBy sorts entries by column index.
func (s *ScannerView) SortBy(col int) {
	if s.SortCol == col {
		s.SortAsc = !s.SortAsc
	} else {
		s.SortCol = col
		s.SortAsc = false
	}

	sort.Slice(s.Entries, func(i, j int) bool {
		var less bool
		switch s.SortCol {
		case 0:
			less = s.Entries[i].Symbol < s.Entries[j].Symbol
		case 1:
			less = s.Entries[i].Price < s.Entries[j].Price
		case 2:
			less = s.Entries[i].ChgPct < s.Entries[j].ChgPct
		case 3:
			less = s.Entries[i].RSI1H < s.Entries[j].RSI1H
		case 4:
			less = s.Entries[i].Bias < s.Entries[j].Bias
		case 5:
			less = s.Entries[i].FVG < s.Entries[j].FVG
		case 6:
			less = s.Entries[i].OIChg < s.Entries[j].OIChg
		case 7:
			less = s.Entries[i].Funding < s.Entries[j].Funding
		case 8:
			less = s.Entries[i].Volume < s.Entries[j].Volume
		default:
			less = s.Entries[i].ChgPct < s.Entries[j].ChgPct
		}
		if s.SortAsc {
			return less
		}
		return !less
	})
}

// View renders the scanner table.
func (s ScannerView) View() string {
	t := s.Theme
	w := s.Width
	if w < 40 {
		w = 120
	}
	innerW := w - 4

	title := t.TableHeader.Render(centerPad("EAX Scanner", innerW))

	colNames := []string{"Symbol", "Price", "Chg%", "RSI(1H)", "Bias", "FVG", "OI Chg%", "Fund", "Volume"}
	header := fmt.Sprintf("%-10s %10s %7s %7s %8s %8s %8s %8s %9s",
		colNames[0], colNames[1], colNames[2], colNames[3],
		colNames[4], colNames[5], colNames[6], colNames[7], colNames[8])
	headerLine := t.TableHeader.Render(truncOrPad(header, innerW))

	var filterLine string
	if s.Filtering {
		filterLine = t.Warning.Render(fmt.Sprintf("  Filter: %s_", s.Filter))
	}

	h := s.Height
	if h < 1 {
		h = 35
	}
	maxRows := h - 5
	if maxRows < 1 {
		maxRows = 1
	}

	entries := s.FilteredEntries()

	var rows []string
	rows = append(rows, title)
	if filterLine != "" {
		rows = append(rows, filterLine)
		maxRows--
	}
	rows = append(rows, headerLine)

	for i, e := range entries {
		if i >= maxRows {
			break
		}

		var priceFmt string
		if e.Price >= 100 {
			priceFmt = fmt.Sprintf("%.2f", e.Price)
		} else if e.Price >= 1 {
			priceFmt = fmt.Sprintf("%.3f", e.Price)
		} else {
			priceFmt = fmt.Sprintf("%.4f", e.Price)
		}

		chgStr := fmt.Sprintf("%+.2f%%", e.ChgPct)
		rsiStr := fmt.Sprintf("%.1f", e.RSI1H)
		oiStr := fmt.Sprintf("%+.1f%%", e.OIChg)
		fundStr := fmt.Sprintf("%.4f", e.Funding)
		volStr := mock.FormatVolume(e.Volume)

		line := fmt.Sprintf("%-10s %10s %7s %7s %8s %8s %8s %8s %9s",
			e.Symbol, priceFmt, chgStr, rsiStr, e.Bias, e.FVG, oiStr, fundStr, volStr)

		if i == s.Cursor {
			rows = append(rows, t.TableRowSel.Render(truncOrPad(line, innerW)))
		} else {
			// Color change column
			sym := t.Normal.Render(fmt.Sprintf("%-10s %10s", e.Symbol, priceFmt))
			var chgStyle string
			if e.ChgPct >= 0 {
				chgStyle = t.PriceUp.Render(fmt.Sprintf(" %7s", chgStr))
			} else {
				chgStyle = t.PriceDown.Render(fmt.Sprintf(" %7s", chgStr))
			}

			// RSI coloring
			var rsiStyle string
			if e.RSI1H > 70 {
				rsiStyle = t.PriceDown.Render(fmt.Sprintf(" %7s", rsiStr))
			} else if e.RSI1H < 30 {
				rsiStyle = t.PriceUp.Render(fmt.Sprintf(" %7s", rsiStr))
			} else {
				rsiStyle = t.Normal.Render(fmt.Sprintf(" %7s", rsiStr))
			}

			// Bias coloring
			var biasStyle string
			switch e.Bias {
			case "Bullish":
				biasStyle = t.PriceUp.Render(fmt.Sprintf(" %8s", e.Bias))
			case "Bearish":
				biasStyle = t.PriceDown.Render(fmt.Sprintf(" %8s", e.Bias))
			default:
				biasStyle = t.Dim.Render(fmt.Sprintf(" %8s", e.Bias))
			}

			// FVG coloring
			var fvgStyle string
			switch e.FVG {
			case "Bullish":
				fvgStyle = t.PriceUp.Render(fmt.Sprintf(" %8s", e.FVG))
			case "Bearish":
				fvgStyle = t.PriceDown.Render(fmt.Sprintf(" %8s", e.FVG))
			default:
				fvgStyle = t.Dim.Render(fmt.Sprintf(" %8s", e.FVG))
			}

			rest := t.Normal.Render(fmt.Sprintf(" %8s %8s %9s", oiStr, fundStr, volStr))
			combined := sym + chgStyle + rsiStyle + biasStyle + fvgStyle + rest
			rows = append(rows, truncOrPad(combined, innerW))
		}
	}

	// Sort indicator
	sortInfo := t.Dim.Render(fmt.Sprintf("  Sort: %s %s  |  [1-9] Sort  [f] Filter  [Enter] Trade",
		colNames[s.SortCol], func() string {
			if s.SortAsc {
				return "▲"
			}
			return "▼"
		}()))
	rows = append(rows, sortInfo)

	content := strings.Join(rows, "\n")

	return t.PanelActive.
		Width(w - 2).
		Height(h).
		Render(content)
}
