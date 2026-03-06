package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ashark-ai-05/tradefox/internal/api"
	"github.com/ashark-ai-05/tradefox/internal/api/handlers"
	"github.com/ashark-ai-05/tradefox/internal/persistence"
	"github.com/ashark-ai-05/tradefox/internal/tui"
	"github.com/ashark-ai-05/tradefox/web"
)

func main() {
	// CLI flags
	configPath := flag.String("config", "", "Config file path (default ~/.tradefox/config.json)")
	paper := flag.Bool("paper", false, "Start in paper trading mode")
	themeName := flag.String("theme", "", "Theme name (dark, light, midnight, matrix, solarized)")
	exchange := flag.String("exchange", "", "Exchange to connect (binance, bybit, etc.)")
	symbol := flag.String("symbol", "BTCUSDT", "Default symbol")
	mockMode := flag.Bool("mock", false, "Force mock data mode (no exchange connection)")
	webMode := flag.Bool("web", false, "Start web UI instead of terminal UI")
	webPort := flag.Int("web-port", 8080, "Port for web UI server")
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
	}

	if *webMode {
		runWeb(*webPort, *symbol, db)
		return
	}

	// --- TUI mode ---

	// Load persisted theme if not specified via flag
	if *themeName == "" && db != nil {
		if saved, err := db.GetSetting("theme"); err == nil && saved != "" {
			*themeName = saved
		}
	}
	if *themeName == "" {
		*themeName = "dark"
	}

	opts := tui.AppOptions{
		ThemeName:  *themeName,
		PaperMode:  *paper,
		MockMode:   *mockMode,
		Exchange:   *exchange,
		Symbol:     *symbol,
		ConfigPath: *configPath,
		DB:         db,
	}

	if opts.MockMode {
		fmt.Fprintln(os.Stderr, "TradeFox: mock data mode (use without --mock for live Binance data)")
	} else {
		fmt.Fprintf(os.Stderr, "TradeFox: connecting to Binance Futures (public) for %s...\n", *symbol)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	app := tui.NewAppWithOptions(opts)

	p := tea.NewProgram(
		app,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

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

func runWeb(port int, symbol string, db *persistence.DB) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	addr := fmt.Sprintf(":%d", port)
	router := api.NewRouter(logger)

	// Chart-specific endpoints
	router.Get("/api/candles", handlers.GetCandles(logger))
	router.Get("/ws/chart", handlers.ServeChartWS(logger))

	// Serve embedded frontend (SPA fallback)
	frontendFS, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not load embedded frontend: %v\n", err)
		os.Exit(1)
	}
	fileServer := http.FileServer(http.FS(frontendFS))
	router.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if len(path) > 0 && path[0] == '/' {
			path = path[1:]
		}
		f, openErr := frontendFS.Open(path)
		if openErr != nil {
			r.URL.Path = "/"
		} else {
			f.Close()
		}
		fileServer.ServeHTTP(w, r)
	}))

	srv := api.NewServer(addr, router)

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		fmt.Fprintf(os.Stderr, "\nTradeFox Web UI: http://localhost:%d\n", port)
		fmt.Fprintf(os.Stderr, "Symbol: %s | Press Ctrl+C to stop\n\n", symbol)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	<-sigCh
	fmt.Fprintln(os.Stderr, "\nShutting down...")

	if db != nil {
		db.Close()
	}

	_ = srv.Close()
}

