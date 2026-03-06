package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ashark-ai-05/tradefox/internal/persistence"
	"github.com/ashark-ai-05/tradefox/internal/tui/components"
	"github.com/ashark-ai-05/tradefox/internal/tui/live"
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
	TabJournal
)

const tabCount = 5

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
	case TabJournal:
		return "Journal"
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

// AppOptions configures the application at startup.
type AppOptions struct {
	ThemeName  string
	PaperMode  bool
	MockMode   bool
	Exchange   string
	Symbol     string
	ConfigPath string
	DB         *persistence.DB
}

// App is the main BubbleTea model.
type App struct {
	theme     theme.Theme
	themeName string
	width     int
	height    int
	activeTab Tab

	header    components.Header
	statusBar components.StatusBar

	trading   views.TradingView
	scanner   views.ScannerView
	positions views.PositionsView
	settings  views.ConfigView
	journal   views.JournalView

	orderEntry views.OrderEntryView
	help       views.HelpView

	paper     *views.PaperEngine
	paperMode bool
	mockMode  bool
	liveMode  bool
	symbol    string

	bridge     *live.LiveDataBridge
	liveCh     chan tea.Msg
	chFullOnce sync.Once

	db *persistence.DB
}

// NewApp creates the main application model with default settings.
func NewApp() App {
	return NewAppWithOptions(AppOptions{ThemeName: "dark"})
}

// NewAppWithOptions creates the main application model with the given options.
func NewAppWithOptions(opts AppOptions) App {
	t := theme.GetTheme(opts.ThemeName)

	paper := views.NewPaperEngine(10000)
	paper.Active = opts.PaperMode
	if opts.DB != nil {
		paper.DB = opts.DB
	}

	// Set up header based on mode
	hdr := components.NewHeader(t)
	if opts.Symbol != "" {
		hdr.Data.Symbol = opts.Symbol
	}
	if opts.Exchange != "" {
		hdr.Data.Exchange = opts.Exchange
	}
	if opts.MockMode {
		hdr.Data.Connected = false
		hdr.Data.Exchange = "Mock"
	}

	configView := views.NewConfigView(t)
	configView.Data.ThemeName = opts.ThemeName
	if opts.Exchange != "" {
		configView.Data.Exchange = opts.Exchange
	}
	configView.Data.ConfigPath = opts.ConfigPath

	journalView := views.NewJournalView(t)
	if opts.DB != nil {
		journalView.RefreshData(opts.DB)
	}

	liveMode := !opts.MockMode
	sym := opts.Symbol
	if sym == "" {
		sym = "BTCUSDT"
	}

	bridge := live.NewLiveDataBridge(nil)
	liveCh := make(chan tea.Msg, 256)

	return App{
		theme:      t,
		themeName:  opts.ThemeName,
		activeTab:  TabTrading,
		header:     hdr,
		statusBar:  components.NewStatusBar(t),
		trading:    views.NewTradingView(t),
		scanner:    views.NewScannerView(t),
		positions:  views.NewPositionsView(t),
		settings:   configView,
		journal:    journalView,
		orderEntry: views.NewOrderEntryView(t),
		help:       views.NewHelpView(t),
		paper:      paper,
		paperMode:  opts.PaperMode,
		mockMode:   opts.MockMode,
		liveMode:   liveMode,
		symbol:     sym,
		bridge:     bridge,
		liveCh:     liveCh,
		db:         opts.DB,
	}
}

// Init implements tea.Model.
func (a App) Init() tea.Cmd {
	cmds := []tea.Cmd{tickCmd()}

	if a.liveMode {
		cmds = append(cmds, a.connectLiveCmd(), a.listenLiveCmd())
	}

	return tea.Batch(cmds...)
}

// connectLiveCmd starts the WebSocket connection in a goroutine.
func (a App) connectLiveCmd() tea.Cmd {
	return func() tea.Msg {
		sym := a.symbol
		ch := a.liveCh
		bridge := a.bridge

		chFullOnce := &a.chFullOnce

		// Register callbacks that push messages to the live channel.
		bridge.SubscribeOrderBook(sym, func(bids, asks []live.OrderBookLevel) {
			select {
			case ch <- OrderBookUpdateMsg{Bids: bids, Asks: asks}:
			default:
				chFullOnce.Do(func() {
					fmt.Fprintf(os.Stderr, "TradeFox: live channel full, dropping messages\n")
				})
			}
		})
		bridge.SubscribeTrades(sym, func(evt live.TradeEvent) {
			select {
			case ch <- TradeUpdateMsg{Trade: evt}:
			default:
				chFullOnce.Do(func() {
					fmt.Fprintf(os.Stderr, "TradeFox: live channel full, dropping messages\n")
				})
			}
		})
		bridge.SubscribeTicker(sym, func(ticker live.TickerUpdate) {
			select {
			case ch <- TickerUpdateMsg{
				Price:       ticker.Price,
				FundingRate: ticker.FundingRate,
				Bid:         ticker.Bid,
				Ask:         ticker.Ask,
			}:
			default:
				chFullOnce.Do(func() {
					fmt.Fprintf(os.Stderr, "TradeFox: live channel full, dropping messages\n")
				})
			}
		})
		bridge.SubscribeCandles(sym, func(candle live.Candle) {
			select {
			case ch <- CandleUpdateMsg{Candle: candle}:
			default:
				chFullOnce.Do(func() {
					fmt.Fprintf(os.Stderr, "TradeFox: live channel full, dropping messages\n")
				})
			}
		})

		ctx := context.Background()
		err := bridge.ConnectPublic(ctx, sym)
		if err != nil {
			return ConnectionStatusMsg{Status: live.StatusError, Connected: false}
		}

		// Start watchlist REST polling in background.
		go pollWatchlist(ctx, ch)

		return ConnectionStatusMsg{Status: live.StatusConnected, Connected: true}
	}
}

// listenLiveCmd listens for messages from the live data channel.
func (a App) listenLiveCmd() tea.Cmd {
	ch := a.liveCh
	return func() tea.Msg {
		return <-ch
	}
}

// pollWatchlist fetches Binance ticker data every 5 seconds.
func pollWatchlist(ctx context.Context, ch chan<- tea.Msg) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Fetch immediately on start.
	fetchAndSend(ctx, ch)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fetchAndSend(ctx, ch)
		}
	}
}

func fetchAndSend(ctx context.Context, ch chan<- tea.Msg) {
	tickers, err := live.FetchWatchlistTickers(ctx)
	if err != nil {
		return
	}

	var data []WatchlistTickerData
	for _, t := range tickers {
		data = append(data, WatchlistTickerData{
			Symbol:   t.Symbol,
			Price:    t.Price,
			Change24: t.Change24,
			Volume:   t.Volume,
		})
	}

	select {
	case ch <- WatchlistUpdateMsg{Tickers: data}:
	default:
	}
}

// Update implements tea.Model.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.header.SetSize(msg.Width)
		a.statusBar.SetSize(msg.Width)
		a.trading.SetSize(msg.Width, msg.Height)
		a.scanner.SetSize(msg.Width, msg.Height)
		a.positions.SetSize(msg.Width, msg.Height)
		a.settings.SetSize(msg.Width, msg.Height)
		a.journal.SetSize(msg.Width, msg.Height)
		return a, nil

	case tickMsg:
		return a, tickCmd()

	case ConnectionStatusMsg:
		a.header.Data.Connected = msg.Connected
		if msg.Connected {
			a.header.Data.Exchange = "Binance"
		}
		return a, nil

	case OrderBookUpdateMsg:
		a.trading.OrderBook.LiveBids = msg.Bids
		a.trading.OrderBook.LiveAsks = msg.Asks
		a.trading.OrderBook.UseLive = true
		// Also update paper engine's best bid/ask
		if a.paper != nil && len(msg.Bids) > 0 && len(msg.Asks) > 0 {
			a.paper.SetBestBidAsk(msg.Bids[0].Price, msg.Asks[0].Price)
		}
		return a, a.listenLiveCmd()

	case TradeUpdateMsg:
		a.trading.Trades.AddLiveTrade(msg.Trade)
		// Check paper pending orders against trade price
		if a.paper != nil && a.paper.Active {
			a.paper.CheckTradePrice(msg.Trade.Symbol, msg.Trade.Price)
		}
		return a, a.listenLiveCmd()

	case TickerUpdateMsg:
		if msg.Price > 0 {
			a.header.Data.Price = msg.Price
			a.header.Data.FundingRate = msg.FundingRate
		}
		if msg.Bid > 0 {
			a.header.Data.Spread = msg.Ask - msg.Bid
		}
		// Update paper position marks
		if a.paper != nil && a.paper.Active && msg.Price > 0 {
			a.paper.UpdateMarkPrice(a.symbol, msg.Price)
		}
		return a, a.listenLiveCmd()

	case CandleUpdateMsg:
		a.trading.Chart.UpdateLiveCandle(msg.Candle)
		return a, a.listenLiveCmd()

	case WatchlistUpdateMsg:
		wt := make([]views.WatchlistTickerData, len(msg.Tickers))
		for i, t := range msg.Tickers {
			wt[i] = views.WatchlistTickerData{
				Symbol:   t.Symbol,
				Price:    t.Price,
				Change24: t.Change24,
				Volume:   t.Volume,
			}
		}
		a.trading.Watchlist.UpdateLivePrices(wt)
		return a, a.listenLiveCmd()

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

// handleQuit cleans up live connections.
func (a *App) handleQuit() {
	if a.bridge != nil {
		a.bridge.Close()
	}
}

func (a App) handleGlobalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		a.handleQuit()
		return a, tea.Quit

	case "?":
		a.help.Visible = true
		return a, nil

	case "tab":
		a.activeTab = (a.activeTab + 1) % tabCount
		return a, nil

	case "shift+tab":
		a.activeTab = (a.activeTab + tabCount - 1) % tabCount
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
	case "5":
		a.activeTab = TabJournal
		return a, nil

	case "t":
		// Cycle theme
		a.themeName = theme.NextTheme(a.themeName)
		a.applyTheme(a.themeName)
		return a, nil

	case "p":
		// Toggle paper mode
		a.paperMode = !a.paperMode
		a.paper.Active = a.paperMode
		return a, nil
	}

	// Tab-specific keys
	switch a.activeTab {
	case TabTrading:
		return a.handleTradingKey(msg)
	case TabScanner:
		return a.handleScannerKey(msg)
	case TabJournal:
		return a.handleJournalKey(msg)
	}

	return a, nil
}

func (a *App) applyTheme(name string) {
	t := theme.GetTheme(name)
	a.theme = t
	a.themeName = name
	a.settings.Data.ThemeName = name
	// Persist theme selection
	if a.db != nil {
		_ = a.db.SaveSetting("theme", name)
	}
}

func (a App) handleTradingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if a.trading.Chart.CrosshairOn {
			if a.trading.Chart.CrosshairPos < len(a.trading.Chart.Candles)-1 {
				a.trading.Chart.CrosshairPos++
			}
		} else if a.trading.Watchlist.Cursor < len(a.trading.Watchlist.Entries)-1 {
			a.trading.Watchlist.Cursor++
			a.updateHeaderFromWatchlist()
		}
	case "k", "up":
		if a.trading.Chart.CrosshairOn {
			if a.trading.Chart.CrosshairPos > 0 {
				a.trading.Chart.CrosshairPos--
			}
		} else if a.trading.Watchlist.Cursor > 0 {
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
	case "<", ",":
		a.trading.Chart.PrevTimeframe()
	case ">", ".":
		a.trading.Chart.NextTimeframe()
	case "c":
		a.trading.Chart.CrosshairOn = !a.trading.Chart.CrosshairOn
		if a.trading.Chart.CrosshairOn {
			a.trading.Chart.CrosshairPos = len(a.trading.Chart.Candles) - 1
		}
	case "l":
		a.trading.Liquidation.Visible = !a.trading.Liquidation.Visible
	case "left", "h":
		if a.trading.Chart.CrosshairOn && a.trading.Chart.CrosshairPos > 0 {
			a.trading.Chart.CrosshairPos--
		}
	case "right":
		if a.trading.Chart.CrosshairOn && a.trading.Chart.CrosshairPos < len(a.trading.Chart.Candles)-1 {
			a.trading.Chart.CrosshairPos++
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
	}
	return a, nil
}

func (a App) handleJournalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if a.journal.Cursor < len(a.journal.Trades)-1 {
			a.journal.Cursor++
		}
	case "k", "up":
		if a.journal.Cursor > 0 {
			a.journal.Cursor--
		}
	case "n":
		a.journal.AddingTrade = true
	case "e":
		a.journal.EditingNotes = true
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
	case TabJournal:
		a.journal.Width = a.width
		a.journal.Height = contentH
		content = a.journal.View()
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
	tabs := []Tab{TabTrading, TabScanner, TabPositions, TabSettings, TabJournal}

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

	// Paper mode indicator
	if a.paperMode {
		paperLabel := lipgloss.NewStyle().
			Background(lipgloss.Color("#b8860b")).
			Foreground(lipgloss.Color("#000000")).
			Bold(true).
			Padding(0, 1).
			Render(" PAPER MODE ")
		rendered = append(rendered, paperLabel)
	}

	// Connection status indicator
	var connDot string
	if a.mockMode {
		connDot = lipgloss.NewStyle().
			Background(t.Colors.StatusBarBg).
			Foreground(t.Colors.FgDim).
			Padding(0, 1).
			Render(" ○ Mock ")
	} else if a.paperMode {
		connDot = lipgloss.NewStyle().
			Background(t.Colors.StatusBarBg).
			Foreground(t.Colors.Warning).
			Padding(0, 1).
			Render(" ◉ Paper ")
	} else {
		connDot = lipgloss.NewStyle().
			Background(t.Colors.StatusBarBg).
			Foreground(t.Colors.PriceUp).
			Padding(0, 1).
			Render(" ● Live ")
	}
	rendered = append(rendered, connDot)

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
