package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ashark-ai-05/tradefox/internal/api"
	"github.com/ashark-ai-05/tradefox/internal/api/handlers"
	"github.com/ashark-ai-05/tradefox/internal/api/ws"
	"github.com/ashark-ai-05/tradefox/internal/config"
	"github.com/ashark-ai-05/tradefox/internal/connector"
	"github.com/ashark-ai-05/tradefox/internal/connector/binance"
	"github.com/ashark-ai-05/tradefox/internal/connector/bitfinex"
	"github.com/ashark-ai-05/tradefox/internal/connector/bitstamp"
	"github.com/ashark-ai-05/tradefox/internal/connector/coinbase"
	"github.com/ashark-ai-05/tradefox/internal/connector/gemini"
	"github.com/ashark-ai-05/tradefox/internal/connector/kraken"
	"github.com/ashark-ai-05/tradefox/internal/connector/kucoin"
	wsconnector "github.com/ashark-ai-05/tradefox/internal/connector/websocket"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
	"github.com/ashark-ai-05/tradefox/internal/execution"
	"github.com/ashark-ai-05/tradefox/internal/liquidation"
	"github.com/ashark-ai-05/tradefox/internal/scanner"
	"github.com/ashark-ai-05/tradefox/internal/logging"
	"github.com/ashark-ai-05/tradefox/internal/plugin"
	"github.com/ashark-ai-05/tradefox/internal/recorder"
	"github.com/ashark-ai-05/tradefox/internal/signals"
	"github.com/ashark-ai-05/tradefox/web"
)

func main() {
	// 1. Setup logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// 2. Load config
	settingsMgr := config.NewManager("")
	if err := settingsMgr.Load(); err != nil {
		logger.Warn("no settings file found, using defaults", slog.String("error", err.Error()))
	}

	serverCfg := settingsMgr.GetServerConfig()

	// Update logging if configured
	if serverCfg.LogFile != "" {
		logLevel := logging.ParseLevel(serverCfg.LogLevel)
		cleanup, err := logging.Setup(logLevel, serverCfg.LogFile)
		if err != nil {
			logger.Warn("failed to setup file logging", slog.String("error", err.Error()))
		} else {
			defer cleanup()
		}
	}

	// 3. Create event bus
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	// 3a. Create context for graceful shutdown (moved up so recorder can use it)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// 3b. Start data recorder if enabled
	if serverCfg.Recorder.Enabled {
		dataDir := serverCfg.Recorder.DataDir
		if dataDir == "" {
			dataDir = "data/recorded"
		}
		rec, err := recorder.New(bus, dataDir, logger)
		if err != nil {
			logger.Error("failed to create recorder", slog.Any("error", err))
		} else {
			rec.Start(ctx)
			defer rec.Stop()
			logger.Info("data recorder enabled", slog.String("dir", dataDir))
		}
	}

	// 4. Create plugin manager
	pluginMgr := plugin.NewManager(bus, settingsMgr, logger)

	// 5. Register connectors from config
	registerConnectors(pluginMgr, bus, settingsMgr, logger)

	// 6. Create WebSocket hub
	hub := ws.NewHub(logger)

	// 6a. Signal engine
	sigEngine := signals.NewEngine(bus, logger)
	sigEngine.OnUpdate(func(symbol string, sigs *signals.SignalSet) {
		hub.Broadcast(ws.WSMessage{
			Type:   "signals",
			Symbol: symbol,
			Data:   sigs,
		})
	})
	sigEngine.Start(ctx)

	// 7. Create data store and subscribe to event bus
	dataStore := handlers.NewDataStore()
	dataStore.SubscribeToEventBus(bus)

	// 8. Create API server with routes
	addr := fmt.Sprintf(":%d", serverCfg.HTTPPort)
	router := api.NewRouter(logger)

	deps := &handlers.Deps{
		DataStore: dataStore,
		PluginMgr: pluginMgr,
		Settings:  settingsMgr,
		Logger:    logger,
	}

	// Mount REST API routes
	router.Get("/api/orderbook/{symbol}", handlers.GetOrderBook(deps))
	router.Get("/api/trades/{symbol}", handlers.GetTrades(deps))
	router.Get("/api/providers", handlers.GetProviders(deps))
	router.Get("/api/studies", handlers.GetStudies(deps))
	router.Get("/api/plugins", handlers.ListPlugins(deps))
	router.Post("/api/plugins/{id}/start", handlers.StartPlugin(deps))
	router.Post("/api/plugins/{id}/stop", handlers.StopPlugin(deps))
	router.Get("/api/settings", handlers.GetSettings(deps))
	router.Put("/api/settings", handlers.UpdateSettings(deps))

	// Execution engine setup
	riskLimits := execution.RiskLimits{
		MaxPositionSize: 100,
		MaxNotional:     1_000_000,
		DailyLossLimit:  10_000,
	}
	riskMgr := execution.NewRiskManager(riskLimits, logger)
	executor := execution.NewExecutor(riskMgr, bus, logger)

	// Register Binance Futures trader
	var traderCfg binance.FuturesTraderConfig
	if err := settingsMgr.GetPluginSettings("binance-futures-trader", &traderCfg); err == nil && traderCfg.APIKey != "" {
		trader := binance.NewFuturesTrader(traderCfg, logger)
		executor.RegisterTrader("binance-futures", trader)
		logger.Info("registered Binance Futures trader")
	}

	presetStore, err := execution.NewPresetStore("data/presets.json")
	if err != nil {
		logger.Warn("failed to initialize preset store", slog.Any("error", err))
	}

	execDeps := &handlers.ExecutionDeps{
		Executor: executor,
		Presets:  presetStore,
	}

	// Execution API routes
	router.Post("/api/orders", handlers.PlaceOrder(execDeps))
	router.Delete("/api/orders/{id}", handlers.CancelOrder(execDeps))
	router.Patch("/api/orders/{id}", handlers.ModifyOrder(execDeps))
	router.Get("/api/orders", handlers.GetOpenOrders(execDeps))
	router.Get("/api/orders/history", handlers.GetOrderHistory(execDeps))
	router.Get("/api/positions", handlers.GetExchangePositions(execDeps))
	router.Post("/api/positions/close", handlers.ClosePosition(execDeps))
	router.Post("/api/orders/presets", handlers.SavePreset(execDeps))
	router.Get("/api/orders/presets", handlers.ListPresets(execDeps))
	router.Delete("/api/orders/presets/{name}", handlers.DeletePreset(execDeps))
	router.Post("/api/killswitch", handlers.KillSwitch(execDeps))
	router.Get("/api/risk/status", handlers.GetRiskStatus(execDeps))

	// Scanner engine setup
	scannerEngine := scanner.NewScannerEngine(bus, logger)
	scannerEngine.Start(ctx)
	logger.Info("scanner engine started")

	scannerDeps := &handlers.ScannerDeps{Engine: scannerEngine}

	// Scanner API routes
	router.Get("/api/scanner/coins", handlers.GetScannerCoins(scannerDeps))
	router.Get("/api/scanner/scatter", handlers.GetScannerScatter(scannerDeps))
	router.Get("/api/scanner/swings", handlers.GetScannerSwings(scannerDeps))
	router.Get("/api/scanner/config", handlers.GetScannerConfig(scannerDeps))
	router.Put("/api/scanner/config", handlers.UpdateScannerConfig(scannerDeps))
	router.Get("/api/scanner/watchlist", handlers.GetScannerWatchlist(scannerDeps))
	router.Put("/api/scanner/watchlist", handlers.UpdateScannerWatchlist(scannerDeps))
	router.Get("/api/scanner/derivatives", handlers.GetScannerDerivatives(scannerDeps))

	// Liquidation engine setup
	liqTracker := liquidation.NewTracker()
	liqEngine := liquidation.NewHeatmapEngine(liqTracker, logger)

	// Subscribe to trades to update prices for heatmap generation
	_, liqTradeCh := bus.Trades.Subscribe(256)
	go func() {
		for t := range liqTradeCh {
			price, _ := t.Price.Float64()
			if price > 0 {
				liqEngine.UpdatePrice(t.Symbol, price)
			}
		}
	}()

	// Push heatmap updates to WebSocket clients
	liqEngine.OnUpdate(func(data liquidation.HeatmapData) {
		hub.Broadcast(ws.WSMessage{
			Type:   "liquidation:heatmap",
			Symbol: data.Symbol,
			Data:   data,
		})
	})

	liqEngine.Start(ctx, 30*time.Second)
	logger.Info("liquidation heatmap engine started")

	liqDeps := &handlers.LiquidationDeps{Engine: liqEngine}

	// Liquidation API routes
	router.Get("/api/liquidations/heatmap", handlers.GetLiquidationHeatmap(liqDeps))
	router.Get("/api/liquidations/feed", handlers.GetLiquidationFeed(liqDeps))
	router.Get("/api/liquidations/stats", handlers.GetLiquidationStats(liqDeps))

	router.Get("/api/signals/{symbol}", func(w http.ResponseWriter, r *http.Request) {
		sym := chi.URLParam(r, "symbol")
		latest := sigEngine.Latest(sym)
		if latest == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(latest)
	})

	// Mount WebSocket endpoint
	router.Get("/ws", hub.ServeWS)

	// Serve embedded frontend (SPA fallback)
	frontendFS, _ := fs.Sub(web.DistFS, "dist")
	fileServer := http.FileServer(http.FS(frontendFS))
	router.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to serve static files first; fall back to index.html for SPA routes.
		f, err := frontendFS.Open(r.URL.Path[1:]) // strip leading /
		if err != nil {
			// Serve index.html for SPA client-side routing.
			r.URL.Path = "/"
		} else {
			f.Close()
		}
		fileServer.ServeHTTP(w, r)
	}))

	// Wire event bus to WebSocket hub (broadcast events to connected browsers)
	go bridgeEventBus(bus, hub)

	srv := api.NewServer(addr, router)

	// 10. Start WebSocket hub
	go hub.Run(ctx)

	// 11. Start plugin manager (all registered plugins)
	if err := pluginMgr.StartAll(ctx); err != nil {
		logger.Warn("some plugins failed to start", slog.String("error", err.Error()))
	}

	// 12. Start HTTP server
	go func() {
		logger.Info("VisualHFT Go server starting", slog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", slog.String("error", err.Error()))
			cancel()
		}
	}()

	// 13. Wait for signal
	<-ctx.Done()
	logger.Info("VisualHFT Go shutting down")

	// 14. Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*1e9) // 10 seconds
	defer shutdownCancel()

	if err := pluginMgr.StopAll(shutdownCtx); err != nil {
		logger.Warn("error stopping plugins", slog.String("error", err.Error()))
	}

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", slog.String("error", err.Error()))
	}

	logger.Info("VisualHFT Go stopped")
}

// registerConnectors reads the "plugins" section of settings.json, looks at each
// entry's "provider" field, and registers the corresponding connector with the
// plugin manager. This is how you get real streaming data — add entries to your
// settings file (default: ~/.visualhft/settings.json).
//
// Example settings.json:
//
//	{
//	  "server": { "httpPort": 8080 },
//	  "plugins": {
//	    "my-binance": {
//	      "provider": "binance",
//	      "symbols": ["BTCUSDT(BTC/USDT)", "ETHUSDT(ETH/USDT)"]
//	    },
//	    "my-kraken": {
//	      "provider": "kraken",
//	      "symbols": ["BTC/USD", "ETH/USD"]
//	    }
//	  }
//	}
func registerConnectors(
	mgr *plugin.Manager,
	bus *eventbus.Bus,
	settings *config.Manager,
	logger *slog.Logger,
) {
	for _, id := range settings.GetAllPluginIDs() {
		// Read the raw JSON to determine the provider type.
		var probe struct {
			Provider string `json:"provider"`
		}
		if err := settings.GetPluginSettings(id, &probe); err != nil {
			logger.Warn("skipping plugin config", slog.String("id", id), slog.Any("error", err))
			continue
		}

		provider := strings.ToLower(strings.TrimSpace(probe.Provider))
		if provider == "" {
			logger.Warn("plugin config missing 'provider' field, skipping", slog.String("id", id))
			continue
		}

		var err error
		switch provider {
		case "binance":
			err = registerBinance(mgr, bus, settings, logger, id)
		case "coinbase":
			err = registerCoinbase(mgr, bus, settings, logger, id)
		case "bitstamp":
			err = registerBitstamp(mgr, bus, logger, id, settings)
		case "gemini":
			err = registerGemini(mgr, bus, logger, id, settings)
		case "kraken":
			err = registerKraken(mgr, bus, logger, id, settings)
		case "kucoin":
			err = registerKuCoin(mgr, bus, logger, id, settings)
		case "bitfinex":
			err = registerBitfinex(mgr, bus, logger, id, settings)
		case "websocket", "generic":
			err = registerWebSocket(mgr, bus, logger, id, settings)
		default:
			logger.Warn("unknown provider type, skipping", slog.String("id", id), slog.String("provider", provider))
			continue
		}

		if err != nil {
			logger.Error("failed to register connector", slog.String("id", id), slog.String("provider", provider), slog.Any("error", err))
		} else {
			logger.Info("registered connector", slog.String("id", id), slog.String("provider", provider))
		}
	}
}

func loadSettings[T any](id string, settings *config.Manager, defaults T) (T, error) {
	raw := json.RawMessage{}
	if err := settings.GetPluginSettings(id, &raw); err != nil {
		return defaults, err
	}
	if err := json.Unmarshal(raw, &defaults); err != nil {
		return defaults, err
	}
	return defaults, nil
}

func registerBinance(mgr *plugin.Manager, bus *eventbus.Bus, settings *config.Manager, logger *slog.Logger, id string) error {
	// Binance connector loads its own settings from the config manager at StartAsync time.
	// We store the config under its plugin ID.
	bc := binance.New(bus, settings, logger)

	// Read user-specified overrides and store them under the plugin's SHA256 ID.
	var userCfg binance.Settings
	if err := settings.GetPluginSettings(id, &userCfg); err == nil && len(userCfg.Symbols) > 0 {
		_ = settings.SetPluginSettings(bc.PluginUniqueID(), userCfg)
	}

	return mgr.Register(bc)
}

func registerCoinbase(mgr *plugin.Manager, bus *eventbus.Bus, settings *config.Manager, logger *slog.Logger, id string) error {
	s, _ := loadSettings(id, settings, coinbase.DefaultSettings())
	base := connector.NewBaseConnector(connector.BaseConnectorConfig{
		Name: "Coinbase", Version: "1.0.0", Description: "Coinbase Advanced Trade market data connector",
		Author: "VisualHFT", ProviderID: s.ProviderID, ProviderName: s.ProviderName,
		Bus: bus, Settings: settings, Logger: logger,
	})
	return mgr.Register(coinbase.New(base, s, logger))
}

func registerBitstamp(mgr *plugin.Manager, bus *eventbus.Bus, logger *slog.Logger, id string, settings *config.Manager) error {
	s, _ := loadSettings(id, settings, bitstamp.DefaultSettings())
	return mgr.Register(bitstamp.New(s, bus, logger))
}

func registerGemini(mgr *plugin.Manager, bus *eventbus.Bus, logger *slog.Logger, id string, settings *config.Manager) error {
	s, _ := loadSettings(id, settings, gemini.DefaultSettings())
	return mgr.Register(gemini.New(s, bus, logger))
}

func registerKraken(mgr *plugin.Manager, bus *eventbus.Bus, logger *slog.Logger, id string, settings *config.Manager) error {
	s, _ := loadSettings(id, settings, kraken.DefaultSettings())
	return mgr.Register(kraken.New(s, bus, logger))
}

func registerKuCoin(mgr *plugin.Manager, bus *eventbus.Bus, logger *slog.Logger, id string, settings *config.Manager) error {
	s, _ := loadSettings(id, settings, kucoin.DefaultSettings())
	return mgr.Register(kucoin.New(s, bus, logger))
}

func registerBitfinex(mgr *plugin.Manager, bus *eventbus.Bus, logger *slog.Logger, id string, settings *config.Manager) error {
	s, _ := loadSettings(id, settings, bitfinex.DefaultSettings())
	return mgr.Register(bitfinex.New(s, bus, logger))
}

func registerWebSocket(mgr *plugin.Manager, bus *eventbus.Bus, logger *slog.Logger, id string, settings *config.Manager) error {
	s, _ := loadSettings(id, settings, wsconnector.DefaultSettings())
	return mgr.Register(wsconnector.New(s, bus, logger))
}

// bridgeEventBus subscribes to all event bus topics and broadcasts to the WebSocket hub.
func bridgeEventBus(bus *eventbus.Bus, hub *ws.Hub) {
	// OrderBooks
	_, obCh := bus.OrderBooks.Subscribe(64)
	go func() {
		for ob := range obCh {
			if ob == nil {
				continue
			}
			hub.Broadcast(ws.WSMessage{
				Type:     "orderbook",
				Symbol:   ob.Symbol,
				Provider: ob.ProviderName,
				Data:     ob,
			})
		}
	}()

	// Trades
	_, trCh := bus.Trades.Subscribe(256)
	go func() {
		for t := range trCh {
			hub.Broadcast(ws.WSMessage{
				Type:     "trade",
				Symbol:   t.Symbol,
				Provider: t.ProviderName,
				Data:     t,
			})
		}
	}()

	// Providers
	_, prCh := bus.Providers.Subscribe(32)
	go func() {
		for p := range prCh {
			hub.Broadcast(ws.WSMessage{
				Type:     "provider",
				Name:     p.ProviderName,
				Provider: p.ProviderName,
				Data:     p,
			})
		}
	}()

	// Studies
	_, stCh := bus.Studies.Subscribe(64)
	go func() {
		for s := range stCh {
			hub.Broadcast(ws.WSMessage{
				Type: "study",
				Data: s,
			})
		}
	}()

	// Positions
	_, posCh := bus.Positions.Subscribe(64)
	go func() {
		for o := range posCh {
			hub.Broadcast(ws.WSMessage{
				Type:   "position",
				Symbol: o.Symbol,
				Data:   o,
			})
		}
	}()

	// Execution events
	_, execCh := bus.Execution.Subscribe(64)
	go func() {
		for e := range execCh {
			hub.Broadcast(ws.WSMessage{
				Type: e.Type,
				Data: e.Data,
			})
		}
	}()
}
