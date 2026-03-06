package theme

import "github.com/charmbracelet/lipgloss"

// ThemeNames lists all available theme names.
var ThemeNames = []string{"dark", "light", "midnight", "matrix", "solarized"}

// GetTheme returns a theme by name. Falls back to Dark.
func GetTheme(name string) Theme {
	switch name {
	case "light":
		return Light()
	case "midnight":
		return Midnight()
	case "matrix":
		return Matrix()
	case "solarized":
		return Solarized()
	default:
		return Dark()
	}
}

// NextTheme returns the next theme name in the cycle.
func NextTheme(current string) string {
	for i, name := range ThemeNames {
		if name == current {
			return ThemeNames[(i+1)%len(ThemeNames)]
		}
	}
	return ThemeNames[0]
}

// Light returns a light theme with white background.
func Light() Theme {
	c := Colors{
		Bg:           lipgloss.Color("#ffffff"),
		Fg:           lipgloss.Color("#333333"),
		FgDim:        lipgloss.Color("#999999"),
		FgBright:     lipgloss.Color("#000000"),
		PriceUp:      lipgloss.Color("#0a8a3e"),
		PriceDown:    lipgloss.Color("#cc2222"),
		Info:         lipgloss.Color("#0077b6"),
		Warning:      lipgloss.Color("#b8860b"),
		HeaderBg:     lipgloss.Color("#e8edf2"),
		HeaderFg:     lipgloss.Color("#1a1a1a"),
		StatusBarBg:  lipgloss.Color("#f0f0f0"),
		StatusBarFg:  lipgloss.Color("#555555"),
		BorderActive: lipgloss.Color("#0077b6"),
		BorderDim:    lipgloss.Color("#cccccc"),
		Highlight:    lipgloss.Color("#ff8800"),
		ModalBg:      lipgloss.Color("#f5f5f5"),
	}
	return buildTheme(c)
}

// Midnight returns a deep navy theme, easier on eyes at night.
func Midnight() Theme {
	c := Colors{
		Bg:           lipgloss.Color("#0d1117"),
		Fg:           lipgloss.Color("#b0b8c4"),
		FgDim:        lipgloss.Color("#484f58"),
		FgBright:     lipgloss.Color("#e6edf3"),
		PriceUp:      lipgloss.Color("#3fb950"),
		PriceDown:    lipgloss.Color("#f85149"),
		Info:         lipgloss.Color("#58a6ff"),
		Warning:      lipgloss.Color("#d29922"),
		HeaderBg:     lipgloss.Color("#161b22"),
		HeaderFg:     lipgloss.Color("#c9d1d9"),
		StatusBarBg:  lipgloss.Color("#1c2128"),
		StatusBarFg:  lipgloss.Color("#8b949e"),
		BorderActive: lipgloss.Color("#388bfd"),
		BorderDim:    lipgloss.Color("#30363d"),
		Highlight:    lipgloss.Color("#ffa657"),
		ModalBg:      lipgloss.Color("#161b22"),
	}
	return buildTheme(c)
}

// Matrix returns an all-green-on-black theme.
func Matrix() Theme {
	c := Colors{
		Bg:           lipgloss.Color("#000000"),
		Fg:           lipgloss.Color("#00ff00"),
		FgDim:        lipgloss.Color("#006600"),
		FgBright:     lipgloss.Color("#00ff00"),
		PriceUp:      lipgloss.Color("#00ff00"),
		PriceDown:    lipgloss.Color("#00aa00"),
		Info:         lipgloss.Color("#00dd00"),
		Warning:      lipgloss.Color("#00bb00"),
		HeaderBg:     lipgloss.Color("#001100"),
		HeaderFg:     lipgloss.Color("#00ff00"),
		StatusBarBg:  lipgloss.Color("#001100"),
		StatusBarFg:  lipgloss.Color("#008800"),
		BorderActive: lipgloss.Color("#00ff00"),
		BorderDim:    lipgloss.Color("#003300"),
		Highlight:    lipgloss.Color("#00ff00"),
		ModalBg:      lipgloss.Color("#001100"),
	}
	return buildTheme(c)
}

// Solarized returns a solarized dark color palette.
func Solarized() Theme {
	c := Colors{
		Bg:           lipgloss.Color("#002b36"),
		Fg:           lipgloss.Color("#839496"),
		FgDim:        lipgloss.Color("#586e75"),
		FgBright:     lipgloss.Color("#fdf6e3"),
		PriceUp:      lipgloss.Color("#859900"),
		PriceDown:    lipgloss.Color("#dc322f"),
		Info:         lipgloss.Color("#268bd2"),
		Warning:      lipgloss.Color("#b58900"),
		HeaderBg:     lipgloss.Color("#073642"),
		HeaderFg:     lipgloss.Color("#93a1a1"),
		StatusBarBg:  lipgloss.Color("#073642"),
		StatusBarFg:  lipgloss.Color("#657b83"),
		BorderActive: lipgloss.Color("#268bd2"),
		BorderDim:    lipgloss.Color("#073642"),
		Highlight:    lipgloss.Color("#cb4b16"),
		ModalBg:      lipgloss.Color("#073642"),
	}
	return buildTheme(c)
}

// buildTheme creates a Theme from a Colors palette (shared logic).
func buildTheme(c Colors) Theme {
	return Theme{
		Colors: c,

		Header: lipgloss.NewStyle().
			Background(c.HeaderBg).
			Foreground(c.HeaderFg).
			Bold(true).
			Padding(0, 1),

		StatusBar: lipgloss.NewStyle().
			Background(c.StatusBarBg).
			Foreground(c.StatusBarFg).
			Padding(0, 1),

		PanelActive: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(c.BorderActive),

		PanelInactive: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(c.BorderDim),

		PriceUp:   lipgloss.NewStyle().Foreground(c.PriceUp),
		PriceDown: lipgloss.NewStyle().Foreground(c.PriceDown),
		Info:      lipgloss.NewStyle().Foreground(c.Info),
		Warning:   lipgloss.NewStyle().Foreground(c.Warning),
		Dim:       lipgloss.NewStyle().Foreground(c.FgDim),
		Bright:    lipgloss.NewStyle().Foreground(c.FgBright).Bold(true),
		Normal:    lipgloss.NewStyle().Foreground(c.Fg),

		ModalBorder: lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(c.BorderActive).
			Background(c.ModalBg).
			Padding(1, 2),

		TableHeader: lipgloss.NewStyle().
			Foreground(c.Info).
			Bold(true),

		TableRow: lipgloss.NewStyle().
			Foreground(c.Fg),

		TableRowSel: lipgloss.NewStyle().
			Foreground(c.FgBright).
			Background(c.HeaderBg).
			Bold(true),

		KeyHint: lipgloss.NewStyle().
			Foreground(c.FgDim),
	}
}
