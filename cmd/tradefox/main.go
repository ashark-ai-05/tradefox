package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"time"
	"os/signal"
	"path/filepath"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/ashark-ai-05/tradefox/internal/api"
	"github.com/ashark-ai-05/tradefox/internal/api/handlers"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
	"github.com/ashark-ai-05/tradefox/internal/nautilus"
	pb "github.com/ashark-ai-05/tradefox/internal/nautilus/proto"
	"github.com/ashark-ai-05/tradefox/internal/persistence"
	"github.com/ashark-ai-05/tradefox/internal/tui"
	"github.com/ashark-ai-05/tradefox/web"
)

func main() {
	// Check for subcommands before flag parsing
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "data":
			handleDataCmd(os.Args[2:])
			return
		case "backtest":
			handleBacktestCmd(os.Args[2:])
			return
		}
	}

	// CLI flags
	configPath := flag.String("config", "", "Config file path (default ~/.tradefox/config.json)")
	paper := flag.Bool("paper", false, "Start in paper trading mode")
	themeName := flag.String("theme", "", "Theme name (dark, light, midnight, matrix, solarized)")
	exchange := flag.String("exchange", "", "Exchange to connect (binance, bybit, etc.)")
	symbol := flag.String("symbol", "BTCUSDT", "Default symbol")
	mockMode := flag.Bool("mock", false, "Force mock data mode (no exchange connection)")
	webMode := flag.Bool("web", false, "Start web UI instead of terminal UI")
	webPort := flag.Int("web-port", 8080, "Port for web UI server")
	nautilusEnabled := flag.Bool("nautilus", false, "Enable NautilusTrader bridge")
	nautilusPort := flag.Int("nautilus-port", 50051, "NautilusTrader gRPC port")
	nautilusAddr := flag.String("nautilus-addr", "localhost", "NautilusTrader gRPC address")
	nautilusNoAutostart := flag.Bool("nautilus-no-autostart", false, "Don't auto-start Nautilus process")
	nautilusPython := flag.String("nautilus-python", "python3", "Python binary for Nautilus")
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

	// Nautilus bridge
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	var bridge *nautilus.NautilusBridge
	if *nautilusEnabled {
		nCfg := nautilus.DefaultConfig()
		nCfg.Enabled = true
		nCfg.GRPCPort = *nautilusPort
		nCfg.GRPCAddress = *nautilusAddr
		nCfg.AutoStart = !*nautilusNoAutostart
		nCfg.PythonPath = *nautilusPython

		bridge = nautilus.NewBridge(nCfg, bus, logger)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := bridge.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: nautilus bridge failed to start: %v\n", err)
		} else {
			defer bridge.Stop()
			if bridge.IsConnected() {
				fmt.Fprintln(os.Stderr, "Nautilus: connected")
			}
		}
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

	// Backtest + data endpoints (connect to Nautilus gRPC)
	nautilusGRPC := "localhost:50051"
	router.Post("/api/backtest/run", handlers.RunBacktest(logger, nautilusGRPC))
	router.Get("/api/backtest/list", handlers.ListBacktests(logger, nautilusGRPC))
	router.Get("/api/backtest/{id}", handlers.GetBacktestResult(logger, nautilusGRPC))
	router.Get("/api/strategies", handlers.GetStrategies(logger))
	router.Post("/api/data/import", handlers.ImportData(logger, nautilusGRPC))
	router.Get("/api/data/available", handlers.GetDataAvailable(logger, nautilusGRPC))

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

func cliGRPCConn() (*grpc.ClientConn, error) {
	addr := os.Getenv("TRADEFOX_GRPC")
	if addr == "" {
		addr = "localhost:50051"
	}
	return grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
}

func handleDataCmd(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: tradefox data <import|list>")
		os.Exit(1)
	}

	switch args[0] {
	case "import":
		fs := flag.NewFlagSet("data import", flag.ExitOnError)
		symbol := fs.String("symbol", "BTCUSDT", "Symbol to import")
		interval := fs.String("interval", "1m", "Bar interval (1m, 5m, 15m, 1h, 4h)")
		fromDate := fs.String("from", "", "Start date (YYYY-MM-DD)")
		toDate := fs.String("to", "", "End date (YYYY-MM-DD)")
		_ = fs.Parse(args[1:])

		var startNs, endNs int64
		if *fromDate != "" {
			t, err := time.Parse("2006-01-02", *fromDate)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: invalid --from date: %v\n", err)
				os.Exit(1)
			}
			startNs = t.UnixNano()
		}
		if *toDate != "" {
			t, err := time.Parse("2006-01-02", *toDate)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: invalid --to date: %v\n", err)
				os.Exit(1)
			}
			endNs = t.UnixNano()
		}
		_ = endNs

		conn, err := cliGRPCConn()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: could not connect to Nautilus: %v\n", err)
			os.Exit(1)
		}
		defer conn.Close()

		client := pb.NewDataServiceClient(conn)
		stream, err := client.ImportData(context.Background(), &pb.ImportRequest{
			Venue:    "BINANCE",
			Symbol:   *symbol,
			DataType: *interval,
			StartNs:  startNs,
			EndNs:    endNs,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("[%.0f%%] %s — %d records\n", msg.PctComplete, msg.Message, msg.RecordsImported)
		}
		fmt.Println("Import complete.")

	case "list":
		conn, err := cliGRPCConn()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: could not connect to Nautilus: %v\n", err)
			os.Exit(1)
		}
		defer conn.Close()

		client := pb.NewDataServiceClient(conn)
		resp, err := client.ListInstruments(context.Background(), &pb.InstrumentFilter{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if len(resp.Instruments) == 0 {
			fmt.Println("No instruments in catalog. Import data first.")
			return
		}
		fmt.Printf("%-30s %-10s %-15s\n", "SYMBOL", "VENUE", "TYPE")
		for _, inst := range resp.Instruments {
			fmt.Printf("%-30s %-10s %-15s\n", inst.Symbol, inst.Venue, inst.InstrumentType)
		}

	default:
		fmt.Printf("Unknown data command: %s\nUsage: tradefox data <import|list>\n", args[0])
		os.Exit(1)
	}
}

func handleBacktestCmd(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: tradefox backtest <run|list|show>")
		os.Exit(1)
	}

	switch args[0] {
	case "run":
		fs := flag.NewFlagSet("backtest run", flag.ExitOnError)
		strategy := fs.String("strategy", "scalp_absorption", "Strategy name")
		symbol := fs.String("symbol", "BTCUSDT", "Symbol")
		capital := fs.String("capital", "10000", "Starting capital")
		riskPct := fs.String("risk", "0.02", "Risk per trade (fraction)")
		venue := fs.String("venue", "BINANCE", "Venue")
		dataType := fs.String("data-type", "", "Bar type string")
		fromDate := fs.String("from", "", "Start date (YYYY-MM-DD)")
		toDate := fs.String("to", "", "End date (YYYY-MM-DD)")
		_ = fs.Parse(args[1:])

		var btStartNs, btEndNs int64
		if *fromDate != "" {
			t, err := time.Parse("2006-01-02", *fromDate)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: invalid --from date: %v\n", err)
				os.Exit(1)
			}
			btStartNs = t.UnixNano()
		}
		if *toDate != "" {
			t, err := time.Parse("2006-01-02", *toDate)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: invalid --to date: %v\n", err)
				os.Exit(1)
			}
			btEndNs = t.UnixNano()
		}

		conn, err := cliGRPCConn()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: could not connect to Nautilus: %v\n", err)
			os.Exit(1)
		}
		defer conn.Close()

		client := pb.NewBacktestServiceClient(conn)
		config := &pb.BacktestConfig{
			StrategyClass: *strategy,
			Venue:         *venue,
			Symbols:       []string{*symbol},
			DataType:      *dataType,
			StartNs:       btStartNs,
			EndNs:         btEndNs,
			StrategyParams: map[string]string{
				"starting_balance":   *capital,
				"risk_per_trade_pct": *riskPct,
				"instrument_id_str":  *symbol + "-PERP." + *venue,
			},
		}

		stream, err := client.RunBacktest(context.Background(), config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		var lastID string
		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			lastID = msg.BacktestId
			fmt.Printf("[%.0f%%] %s: %s\n", msg.PctComplete, msg.Status, msg.Message)
		}

		if lastID != "" {
			result, err := client.GetBacktestResult(context.Background(), &pb.BacktestId{Id: lastID})
			if err == nil {
				fmt.Println("\n=== Backtest Results ===")
				fmt.Printf("ID:            %s\n", result.Id)
				fmt.Printf("Total Return:  %.2f%%\n", result.TotalReturn*100)
				fmt.Printf("Sharpe Ratio:  %.2f\n", result.SharpeRatio)
				fmt.Printf("Max Drawdown:  %.2f%%\n", result.MaxDrawdown*100)
				fmt.Printf("Win Rate:      %.1f%%\n", result.WinRate*100)
				fmt.Printf("Profit Factor: %.2f\n", result.ProfitFactor)
				fmt.Printf("Total Trades:  %d\n", result.TotalTrades)
			}
		}

	case "list":
		conn, err := cliGRPCConn()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: could not connect to Nautilus: %v\n", err)
			os.Exit(1)
		}
		defer conn.Close()

		client := pb.NewBacktestServiceClient(conn)
		resp, err := client.ListBacktests(context.Background(), &pb.Empty{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if len(resp.Backtests) == 0 {
			fmt.Println("No backtests found.")
			return
		}
		fmt.Printf("%-10s %-20s %-10s %10s %8s\n", "ID", "STRATEGY", "STATUS", "RETURN", "SHARPE")
		for _, bt := range resp.Backtests {
			fmt.Printf("%-10s %-20s %-10s %9.2f%% %8.2f\n",
				bt.Id, bt.StrategyClass, bt.Status,
				bt.TotalReturn*100, bt.SharpeRatio)
		}

	case "show":
		if len(args) < 2 {
			fmt.Println("Usage: tradefox backtest show <ID>")
			os.Exit(1)
		}
		id := args[1]

		conn, err := cliGRPCConn()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: could not connect to Nautilus: %v\n", err)
			os.Exit(1)
		}
		defer conn.Close()

		client := pb.NewBacktestServiceClient(conn)
		result, err := client.GetBacktestResult(context.Background(), &pb.BacktestId{Id: id})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		out, _ := json.MarshalIndent(map[string]interface{}{
			"id":            result.Id,
			"total_return":  result.TotalReturn,
			"sharpe_ratio":  result.SharpeRatio,
			"max_drawdown":  result.MaxDrawdown,
			"win_rate":      result.WinRate,
			"profit_factor": result.ProfitFactor,
			"total_trades":  result.TotalTrades,
			"trades_count":  len(result.Trades),
			"equity_points": len(result.EquityCurve),
		}, "", "  ")
		fmt.Println(string(out))

	default:
		fmt.Printf("Unknown backtest command: %s\nUsage: tradefox backtest <run|list|show>\n", args[0])
		os.Exit(1)
	}
}

