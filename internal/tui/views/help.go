package views

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ashark-ai-05/tradefox/internal/tui/theme"
)

// HelpView is the help overlay modal.
type HelpView struct {
	Visible bool
	Width   int
	Height  int
	Theme   theme.Theme
}

// NewHelpView creates a new help overlay.
func NewHelpView(t theme.Theme) HelpView {
	return HelpView{Theme: t}
}

// View renders the help overlay.
func (h HelpView) View() string {
	if !h.Visible {
		return ""
	}

	t := h.Theme
	modalW := 60

	title := t.Bright.Render("  TradeFox Keyboard Shortcuts")

	sections := []struct {
		name  string
		keys  [][2]string
	}{
		{
			"Global",
			[][2]string{
				{"Tab / Shift+Tab", "Switch tabs"},
				{"1-4", "Jump to tab"},
				{"?", "Toggle help"},
				{"q / Ctrl+C", "Quit"},
				{"/", "Search / Filter"},
			},
		},
		{
			"Trading",
			[][2]string{
				{"b", "Buy order entry"},
				{"s", "Sell order entry"},
				{"x", "Close selected position"},
				{"j/k or Up/Down", "Navigate watchlist"},
				{"Enter", "Select instrument"},
				{"[ / ]", "Decrease/increase depth"},
			},
		},
		{
			"Scanner",
			[][2]string{
				{"j/k or Up/Down", "Navigate coins"},
				{"1-9", "Sort by column"},
				{"f", "Toggle filter"},
				{"Enter", "Trade selected coin"},
			},
		},
		{
			"Order Entry",
			[][2]string{
				{"Tab", "Next field"},
				{"Shift+Tab", "Previous field"},
				{"Enter", "Submit order"},
				{"Esc", "Cancel"},
			},
		},
	}

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")

	for _, sec := range sections {
		lines = append(lines, t.Info.Bold(true).Render("  "+sec.name))
		for _, kv := range sec.keys {
			key := t.Warning.Render(padRight(kv[0], 20))
			desc := t.Normal.Render(kv[1])
			lines = append(lines, "    "+key+"  "+desc)
		}
		lines = append(lines, "")
	}

	lines = append(lines, t.Dim.Render("  Press any key to dismiss"))

	content := strings.Join(lines, "\n")

	modal := t.ModalBorder.
		Width(modalW).
		Render(content)

	screenW := h.Width
	screenH := h.Height
	if screenW < 1 {
		screenW = 120
	}
	if screenH < 1 {
		screenH = 40
	}

	modalH := lipgloss.Height(modal)
	modalActualW := lipgloss.Width(modal)

	padLeft := (screenW - modalActualW) / 2
	padTop := (screenH - modalH) / 3
	if padLeft < 0 {
		padLeft = 0
	}
	if padTop < 0 {
		padTop = 0
	}

	return lipgloss.NewStyle().
		MarginLeft(padLeft).
		MarginTop(padTop).
		Render(modal)
}

func padRight(s string, w int) string {
	if len(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(s))
}
