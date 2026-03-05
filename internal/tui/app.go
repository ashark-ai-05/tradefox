package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ashark-ai-05/tradefox/internal/tui/components"
	"github.com/ashark-ai-05/tradefox/internal/tui/theme"
	"github.com/ashark-ai-05/tradefox/internal/tui/views"
)

// Tab represents a main navigation tab.
type Tab int

const (
	TabTrading  Tab = iota
	TabScanner
	TabPositions
	TabSettings
)

func (t Tab) String() string {
	switch t {
	case TabTrading:
		return "Trading"
	case TabScanner:
		return "Scanner"
	case TabPositions:
		return "Positions"
	case TabSettings:
		return "Settings"
	default:
		return "Trading"
	}
}

// tickMsg is sent periodically to update the clock.
type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// App is the main BubbleTea model.
type App struct {
	theme     theme.Theme
	width     int
	height    int
	activeTab Tab

	header    components.Header
	statusBar components.StatusBar

	trading   views.TradingView
	scanner   views.ScannerView
	positions views.PositionsView
	settings  views.SettingsView

	orderEntry views.OrderEntryView
	help       views.HelpView
}

// NewApp creates the main application model.
func NewApp() App {
	t := theme.Dark()
	return App{
		theme:      t,
		activeTab:  TabTrading,
		header:     components.NewHeader(t),
		statusBar:  components.NewStatusBar(t),
		trading:    views.NewTradingView(t),
		scanner:    views.NewScannerView(t),
		positions:  views.NewPositionsView(t),
		settings:   views.NewSettingsView(t),
		orderEntry: views.NewOrderEntryView(t),
		help:       views.NewHelpView(t),
	}
}

// Init implements tea.Model.
func (a App) Init() tea.Cmd {
	return tickCmd()
}

// Update implements tea.Model.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		return a, nil

	case tickMsg:
		return a, tickCmd()

	case tea.KeyMsg:
		// Help overlay captures all keys
		if a.help.Visible {
			a.help.Visible = false
			return a, nil
		}

		// Order entry modal captures keys
		if a.orderEntry.Visible {
			return a.handleOrderEntryKey(msg)
		}

		// Scanner filter mode captures keys
		if a.activeTab == TabScanner && a.scanner.Filtering {
			return a.handleScannerFilterKey(msg)
		}

		return a.handleGlobalKey(msg)
	}

	return a, nil
}

func (a App) handleGlobalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return a, tea.Quit

	case "?":
		a.help.Visible = true
		return a, nil

	case "tab":
		a.activeTab = (a.activeTab + 1) % 4
		return a, nil

	case "shift+tab":
		a.activeTab = (a.activeTab + 3) % 4
		return a, nil

	case "1":
		a.activeTab = TabTrading
		return a, nil
	case "2":
		a.activeTab = TabScanner
		return a, nil
	case "3":
		a.activeTab = TabPositions
		return a, nil
	case "4":
		a.activeTab = TabSettings
		return a, nil
	}

	// Tab-specific keys
	switch a.activeTab {
	case TabTrading:
		return a.handleTradingKey(msg)
	case TabScanner:
		return a.handleScannerKey(msg)
	}

	return a, nil
}

func (a App) handleTradingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if a.trading.Watchlist.Cursor < len(a.trading.Watchlist.Entries)-1 {
			a.trading.Watchlist.Cursor++
			a.updateHeaderFromWatchlist()
		}
	case "k", "up":
		if a.trading.Watchlist.Cursor > 0 {
			a.trading.Watchlist.Cursor--
			a.updateHeaderFromWatchlist()
		}
	case "b":
		entry := a.trading.Watchlist.SelectedEntry()
		a.orderEntry.Show(true, entry.Symbol, entry.Bid)
	case "s":
		entry := a.trading.Watchlist.SelectedEntry()
		a.orderEntry.Show(false, entry.Symbol, entry.Ask)
	case "enter":
		a.updateHeaderFromWatchlist()
	case "[":
		if a.trading.OrderBook.Depth > 5 {
			a.trading.OrderBook.Depth--
		}
	case "]":
		if a.trading.OrderBook.Depth < 25 {
			a.trading.OrderBook.Depth++
		}
	}
	return a, nil
}

func (a App) handleScannerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		filtered := a.scanner.FilteredEntries()
		if a.scanner.Cursor < len(filtered)-1 {
			a.scanner.Cursor++
		}
	case "k", "up":
		if a.scanner.Cursor > 0 {
			a.scanner.Cursor--
		}
	case "f":
		a.scanner.Filtering = true
		a.scanner.Filter = ""
	case "enter":
		// Switch to trading tab for selected coin
		a.activeTab = TabTrading
	case "1":
		a.scanner.SortBy(0)
	case "2":
		a.scanner.SortBy(1)
	case "3":
		a.scanner.SortBy(2)
	case "4":
		a.scanner.SortBy(3)
	case "5":
		a.scanner.SortBy(4)
	case "6":
		a.scanner.SortBy(5)
	case "7":
		a.scanner.SortBy(6)
	case "8":
		a.scanner.SortBy(7)
	case "9":
		a.scanner.SortBy(8)
	}
	return a, nil
}

func (a App) handleScannerFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.scanner.Filtering = false
		a.scanner.Filter = ""
	case "enter":
		a.scanner.Filtering = false
	case "backspace":
		if len(a.scanner.Filter) > 0 {
			a.scanner.Filter = a.scanner.Filter[:len(a.scanner.Filter)-1]
		}
	default:
		if len(msg.String()) == 1 {
			a.scanner.Filter += msg.String()
			a.scanner.Cursor = 0
		}
	}
	return a, nil
}

func (a App) handleOrderEntryKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.orderEntry.Hide()
	case "tab":
		a.orderEntry.NextField()
	case "shift+tab":
		a.orderEntry.PrevField()
	case "enter":
		if a.orderEntry.ActiveField == 0 {
			a.orderEntry.CycleOrderType()
		} else {
			a.orderEntry.Confirmed = true
		}
	case "backspace":
		a.orderEntry.Backspace()
	default:
		ch := msg.String()
		if len(ch) == 1 && ((ch[0] >= '0' && ch[0] <= '9') || ch[0] == '.') {
			a.orderEntry.TypeChar(ch)
		}
	}
	return a, nil
}

func (a *App) updateHeaderFromWatchlist() {
	entry := a.trading.Watchlist.SelectedEntry()
	a.header.Data.Symbol = entry.Symbol
	a.header.Data.Price = entry.Price
	a.header.Data.Change24 = entry.Change24
	a.header.Data.Spread = entry.Spread()
}

// View implements tea.Model.
func (a App) View() string {
	if a.width == 0 {
		return "Loading TradeFox..."
	}

	// Header
	a.header.Width = a.width
	headerView := a.header.View()

	// Tab bar
	tabBar := a.renderTabBar()

	// Status bar
	a.statusBar.Width = a.width
	statusView := a.statusBar.View()

	// Main content area
	contentH := a.height - lipgloss.Height(headerView) - lipgloss.Height(tabBar) - lipgloss.Height(statusView)
	if contentH < 5 {
		contentH = 5
	}

	var content string
	switch a.activeTab {
	case TabTrading:
		a.trading.Width = a.width
		a.trading.Height = contentH
		content = a.trading.View()
	case TabScanner:
		a.scanner.Width = a.width
		a.scanner.Height = contentH
		content = a.scanner.View()
	case TabPositions:
		a.positions.Width = a.width
		a.positions.Height = contentH
		content = a.positions.View()
	case TabSettings:
		a.settings.Width = a.width
		a.settings.Height = contentH
		content = a.settings.View()
	}

	// Compose full screen
	screen := lipgloss.JoinVertical(lipgloss.Left,
		headerView,
		tabBar,
		content,
		statusView,
	)

	// Overlay modals
	if a.orderEntry.Visible {
		a.orderEntry.Width = a.width
		a.orderEntry.Height = a.height
		overlay := a.orderEntry.View()
		if overlay != "" {
			return overlay
		}
	}

	if a.help.Visible {
		a.help.Width = a.width
		a.help.Height = a.height
		overlay := a.help.View()
		if overlay != "" {
			return overlay
		}
	}

	return screen
}

func (a App) renderTabBar() string {
	t := a.theme
	tabs := []Tab{TabTrading, TabScanner, TabPositions, TabSettings}

	var rendered []string
	for _, tab := range tabs {
		label := " " + tab.String() + " "
		if tab == a.activeTab {
			style := lipgloss.NewStyle().
				Background(t.Colors.BorderActive).
				Foreground(t.Colors.FgBright).
				Bold(true).
				Padding(0, 1)
			rendered = append(rendered, style.Render(label))
		} else {
			style := lipgloss.NewStyle().
				Background(t.Colors.StatusBarBg).
				Foreground(t.Colors.FgDim).
				Padding(0, 1)
			rendered = append(rendered, style.Render(label))
		}
	}

	bar := lipgloss.JoinHorizontal(lipgloss.Center, rendered...)

	// Fill remaining width
	barW := lipgloss.Width(bar)
	remaining := a.width - barW
	if remaining > 0 {
		filler := lipgloss.NewStyle().
			Background(t.Colors.StatusBarBg).
			Render(strings.Repeat(" ", remaining))
		bar += filler
	}

	return bar
}
