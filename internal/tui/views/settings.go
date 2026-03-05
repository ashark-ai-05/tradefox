package views

import (
	"strings"

	"github.com/ashark-ai-05/tradefox/internal/tui/theme"
)

// SettingsView is a placeholder settings tab.
type SettingsView struct {
	Width  int
	Height int
	Theme  theme.Theme
}

// NewSettingsView creates a settings view.
func NewSettingsView(t theme.Theme) SettingsView {
	return SettingsView{Theme: t}
}

// View renders the settings placeholder.
func (s SettingsView) View() string {
	t := s.Theme
	w := s.Width
	if w < 20 {
		w = 120
	}
	h := s.Height
	if h < 5 {
		h = 35
	}

	var lines []string
	lines = append(lines, "")
	lines = append(lines, t.Bright.Render(centerPad("Settings", w-4)))
	lines = append(lines, "")
	lines = append(lines, t.Dim.Render(centerPad("Settings panel coming in Phase 2", w-4)))
	lines = append(lines, "")
	lines = append(lines, t.Normal.Render("  Exchange:     Binance Futures"))
	lines = append(lines, t.Normal.Render("  Theme:        Dark"))
	lines = append(lines, t.Normal.Render("  OB Depth:     15 levels"))
	lines = append(lines, t.Normal.Render("  Risk Limit:   5% per trade"))
	lines = append(lines, t.Normal.Render("  Auto-close:   Enabled"))
	lines = append(lines, "")
	lines = append(lines, t.Dim.Render(centerPad("Configuration via --config flag or ~/.tradefox/config.json", w-4)))

	content := strings.Join(lines, "\n")

	return t.PanelActive.
		Width(w - 2).
		Height(h).
		Render(content)
}
