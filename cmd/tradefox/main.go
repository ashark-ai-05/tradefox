package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ashark-ai-05/tradefox/internal/persistence"
	"github.com/ashark-ai-05/tradefox/internal/tui"
)

func main() {
	// CLI flags
	configPath := flag.String("config", "", "Config file path (default ~/.tradefox/config.json)")
	paper := flag.Bool("paper", false, "Start in paper trading mode")
	themeName := flag.String("theme", "", "Theme name (dark, light, midnight, matrix, solarized)")
	exchange := flag.String("exchange", "", "Exchange to connect (binance, bybit, etc.)")
	symbol := flag.String("symbol", "BTCUSDT", "Default symbol")
	mockMode := flag.Bool("mock", false, "Force mock data mode (no exchange connection)")
	flag.Parse()

	// Ensure ~/.tradefox/ directory exists
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	tradefoxDir := filepath.Join(homeDir, ".tradefox")
	if err := os.MkdirAll(tradefoxDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not create %s: %v\n", tradefoxDir, err)
	}

	// Resolve config path
	if *configPath == "" {
		*configPath = filepath.Join(tradefoxDir, "config.json")
	}

	// Initialize SQLite database
	dbPath := filepath.Join(tradefoxDir, "journal.db")
	db, err := persistence.NewDB(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not open journal database: %v\n", err)
		// Continue without persistence
	}

	// Load persisted theme if not specified via flag
	if *themeName == "" && db != nil {
		if saved, err := db.GetSetting("theme"); err == nil && saved != "" {
			*themeName = saved
		}
	}
	if *themeName == "" {
		*themeName = "dark"
	}

	// Build app options
	opts := tui.AppOptions{
		ThemeName:  *themeName,
		PaperMode:  *paper,
		MockMode:   *mockMode || *exchange == "",
		Exchange:   *exchange,
		Symbol:     *symbol,
		ConfigPath: *configPath,
		DB:         db,
	}

	if opts.MockMode && *exchange == "" {
		fmt.Fprintln(os.Stderr, "TradeFox: no exchange configured, using mock data")
	}

	// Graceful shutdown on SIGINT/SIGTERM
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	app := tui.NewAppWithOptions(opts)

	p := tea.NewProgram(
		app,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	// Run shutdown listener in background
	go func() {
		<-sigCh
		if db != nil {
			db.Close()
		}
		p.Quit()
	}()

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TradeFox: %v\n", err)
		if db != nil {
			db.Close()
		}
		os.Exit(1)
	}

	if db != nil {
		db.Close()
	}
}
