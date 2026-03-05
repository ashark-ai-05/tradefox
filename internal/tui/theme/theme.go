package theme

import "github.com/charmbracelet/lipgloss"

// Colors defines the color palette for the TUI.
type Colors struct {
	Bg           lipgloss.Color
	Fg           lipgloss.Color
	FgDim        lipgloss.Color
	FgBright     lipgloss.Color
	PriceUp      lipgloss.Color
	PriceDown    lipgloss.Color
	Info         lipgloss.Color
	Warning      lipgloss.Color
	HeaderBg     lipgloss.Color
	HeaderFg     lipgloss.Color
	StatusBarBg  lipgloss.Color
	StatusBarFg  lipgloss.Color
	BorderActive lipgloss.Color
	BorderDim    lipgloss.Color
	Highlight    lipgloss.Color
	ModalBg      lipgloss.Color
}

// Theme holds all styles for the application.
type Theme struct {
	Colors Colors

	Header    lipgloss.Style
	StatusBar lipgloss.Style

	PanelActive   lipgloss.Style
	PanelInactive lipgloss.Style

	PriceUp   lipgloss.Style
	PriceDown lipgloss.Style
	Info      lipgloss.Style
	Warning   lipgloss.Style
	Dim       lipgloss.Style
	Bright    lipgloss.Style
	Normal    lipgloss.Style

	ModalBorder lipgloss.Style
	TableHeader lipgloss.Style
	TableRow    lipgloss.Style
	TableRowSel lipgloss.Style

	KeyHint lipgloss.Style
}

// Dark returns the default dark theme.
func Dark() Theme {
	c := Colors{
		Bg:           lipgloss.Color("#0a0a0a"),
		Fg:           lipgloss.Color("#c0c0c0"),
		FgDim:        lipgloss.Color("#606060"),
		FgBright:     lipgloss.Color("#ffffff"),
		PriceUp:      lipgloss.Color("#00d775"),
		PriceDown:    lipgloss.Color("#ff4d4d"),
		Info:         lipgloss.Color("#00bcd4"),
		Warning:      lipgloss.Color("#ffc107"),
		HeaderBg:     lipgloss.Color("#0d1b2a"),
		HeaderFg:     lipgloss.Color("#e0e0e0"),
		StatusBarBg:  lipgloss.Color("#1a1a1a"),
		StatusBarFg:  lipgloss.Color("#909090"),
		BorderActive: lipgloss.Color("#4488cc"),
		BorderDim:    lipgloss.Color("#333333"),
		Highlight:    lipgloss.Color("#ffff00"),
		ModalBg:      lipgloss.Color("#141414"),
	}

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
			Background(lipgloss.Color("#1a2a3a")).
			Bold(true),

		KeyHint: lipgloss.NewStyle().
			Foreground(c.FgDim),
	}
}
