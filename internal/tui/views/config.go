package views

import (
	"fmt"
	"strings"

	"github.com/ashark-ai-05/tradefox/internal/tui/theme"
)

// ConfigData holds the current configuration state displayed in settings.
type ConfigData struct {
	Exchange        string
	APIKeySet       bool
	DefaultSymbol   string
	MaxPositionSize float64
	DailyLossCap    float64
	KillSwitch      float64
	ScannerCoins    []string
	ThemeName       string
	PaperCapital    float64
	ConfigPath      string
}

// ConfigView is the settings configuration panel (Tab 4).
type ConfigView struct {
	Data     ConfigData
	Cursor   int
	Editing  bool
	Width    int
	Height   int
	Theme    theme.Theme
}

// NewConfigView creates a settings view with defaults.
func NewConfigView(t theme.Theme) ConfigView {
	return ConfigView{
		Theme: t,
		Data: ConfigData{
			Exchange:        "binance",
			DefaultSymbol:   "BTCUSDT",
			MaxPositionSize: 5.0,
			DailyLossCap:    500.0,
			KillSwitch:      2000.0,
			ThemeName:       "dark",
			PaperCapital:    10000.0,
			ConfigPath:      "~/.tradefox/config.json",
		},
	}
}

// View renders the configuration panel.
func (c ConfigView) View() string {
	t := c.Theme
	w := c.Width
	if w < 40 {
		w = 120
	}
	innerW := w - 4
	h := c.Height
	if h < 10 {
		h = 35
	}

	var lines []string
	lines = append(lines, t.TableHeader.Render(centerPad("Settings", innerW)))
	lines = append(lines, "")

	items := []struct {
		label string
		value string
	}{
		{"Exchange", c.Data.Exchange},
		{"API Key", c.apiKeyDisplay()},
		{"Default Symbol", c.Data.DefaultSymbol},
		{"Theme", c.Data.ThemeName},
		{"Max Position Size", fmt.Sprintf("%.1f%%", c.Data.MaxPositionSize)},
		{"Daily Loss Cap", fmt.Sprintf("$%.0f", c.Data.DailyLossCap)},
		{"Kill Switch", fmt.Sprintf("$%.0f", c.Data.KillSwitch)},
		{"Paper Capital", fmt.Sprintf("$%.0f", c.Data.PaperCapital)},
		{"Scanner Coins", fmt.Sprintf("%d coins", len(c.Data.ScannerCoins))},
		{"Config File", c.Data.ConfigPath},
	}

	for i, item := range items {
		prefix := "  "
		if i == c.Cursor {
			prefix = "> "
		}

		label := t.Info.Render(fmt.Sprintf("%-20s", item.label))
		value := t.Normal.Render(item.value)

		if i == c.Cursor {
			line := t.TableRowSel.Render(truncOrPad(prefix+item.label+"  "+item.value, innerW))
			lines = append(lines, line)
		} else {
			lines = append(lines, prefix+label+"  "+value)
		}
	}

	lines = append(lines, "")
	lines = append(lines, t.Dim.Render(strings.Repeat("─", innerW)))
	lines = append(lines, "")

	// Theme preview section
	lines = append(lines, t.Info.Bold(true).Render("  Available Themes:"))
	for _, name := range theme.ThemeNames {
		marker := "  "
		if name == c.Data.ThemeName {
			marker = "* "
		}
		lines = append(lines, t.Normal.Render("    "+marker+name))
	}

	lines = append(lines, "")
	lines = append(lines, t.Dim.Render("  [j/k] Navigate  [Enter] Edit  [t] Cycle Theme  [Esc] Cancel"))
	lines = append(lines, t.Dim.Render(fmt.Sprintf("  Config: %s", c.Data.ConfigPath)))

	content := strings.Join(lines, "\n")
	return t.PanelActive.
		Width(w - 2).
		Height(h).
		Render(content)
}

func (c ConfigView) apiKeyDisplay() string {
	if c.Data.APIKeySet {
		return "********** (set)"
	}
	return "(not set)"
}
