package scanner

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

// ScannerEngine coordinates scanning of all configured symbols.
type ScannerEngine struct {
	config     ScannerConfig
	bus        *eventbus.Bus
	logger     *slog.Logger
	results    map[string]*CoinScan
	watchlist  []string
	mu         sync.RWMutex
	klineCache *KlineCache
	configPath string
	watchPath  string
}

// NewScannerEngine creates a new scanner engine.
func NewScannerEngine(bus *eventbus.Bus, logger *slog.Logger) *ScannerEngine {
	homeDir, _ := os.UserHomeDir()
	configDir := filepath.Join(homeDir, ".visualhft")
	_ = os.MkdirAll(configDir, 0755)

	e := &ScannerEngine{
		config:     loadConfigFromDisk(configDir),
		bus:        bus,
		logger:     logger,
		results:    make(map[string]*CoinScan),
		klineCache: NewKlineCache(),
		configPath: filepath.Join(configDir, "scanner.json"),
		watchPath:  filepath.Join(configDir, "watchlist.json"),
	}

	e.watchlist = loadWatchlistFromDisk(e.watchPath)
	return e
}

// Start begins the scanner loop.
func (e *ScannerEngine) Start(ctx context.Context) {
	go e.scanLoop(ctx)
}

func (e *ScannerEngine) scanLoop(ctx context.Context) {
	// Run immediately on start
	e.scanAll(ctx)

	ticker := time.NewTicker(e.config.ScanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.scanAll(ctx)
		}
	}
}

func (e *ScannerEngine) scanAll(ctx context.Context) {
	e.mu.RLock()
	symbols := make([]string, len(e.config.Symbols))
	copy(symbols, e.config.Symbols)
	market := e.config.Market
	e.mu.RUnlock()

	baseURL := BaseURL(market)
	klinePath := KlinePath(market)
	tickerPath := TickerPath(market)

	// Rate limit: max 10 concurrent fetches
	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup

	for _, sym := range symbols {
		wg.Add(1)
		go func(symbol string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			scan := e.scanSymbol(ctx, baseURL, klinePath, tickerPath, symbol, market)
			if scan != nil {
				e.mu.Lock()
				e.results[symbol] = scan
				e.mu.Unlock()
			}
		}(sym)
	}

	wg.Wait()

	// Emit scanner:update event
	if e.bus != nil {
		e.bus.Execution.Publish(eventbus.ExecutionEvent{
			Type: "scanner:update",
			Data: e.GetResults(),
		})
	}

	e.logger.Info("scanner scan complete", slog.Int("symbols", len(symbols)))
}

func (e *ScannerEngine) scanSymbol(ctx context.Context, baseURL, klinePath, tickerPath, symbol, market string) *CoinScan {
	// Fetch all needed timeframes
	timeframes := []string{"5m", "15m", "1h", "4h", "12h", "1d", "1w", "1M"}
	klines := make(map[string][]Candle)

	for _, tf := range timeframes {
		// Check cache first
		if cached, ok := e.klineCache.get(symbol, tf); ok {
			klines[tf] = cached
			continue
		}

		candles, err := FetchKlines(ctx, baseURL, klinePath, symbol, tf, 100)
		if err != nil {
			e.logger.Debug("failed to fetch klines",
				slog.String("symbol", symbol),
				slog.String("tf", tf),
				slog.Any("error", err))
			continue
		}

		klines[tf] = candles
		e.klineCache.set(symbol, tf, candles, intervalTTL(tf))
	}

	// Fetch 24h change and price
	change24h, price, err := Fetch24hChange(ctx, baseURL, tickerPath, symbol)
	if err != nil {
		e.logger.Debug("failed to fetch ticker", slog.String("symbol", symbol), slog.Any("error", err))
		// Try to get price from 1h candles
		if candles, ok := klines["1h"]; ok && len(candles) > 0 {
			price = candles[len(candles)-1].Close
		}
	}

	if price == 0 {
		return nil
	}

	// Compute RSI
	rsiValues, rsiHistory := ComputeRSIForTimeframes(klines)
	rsiState := AggregateRSIState(rsiValues)

	// Compute bias for each timeframe
	bias1H := CalcBias(klines["1h"])
	bias4H := CalcBias(klines["4h"])
	biasD := CalcBias(klines["1d"])
	biasW := CalcBias(klines["1w"])

	// Compute FVGs across daily and 4h
	var allFVGs []FVG
	for _, tf := range FVGTimeframes {
		if candles, ok := klines[tf]; ok {
			fvgs := DetectFVGs(candles, tf)
			allFVGs = append(allFVGs, fvgs...)
		}
	}
	nextFVG := FindNearestFVG(allFVGs, price)

	// Compute weekly pivots
	var pivotWidth string
	var proximity ProximityResult
	if weekly, ok := klines["1w"]; ok && len(weekly) >= 2 {
		wh, wl, wc := ExtractWeeklyHLC(weekly)
		weeklyPivots := CalcWeeklyPivots(wh, wl, wc)
		pivotWidth = ClassifyPivotWidth(weeklyPivots.S1, weeklyPivots.R1, price)
		proximity = FindNearestPivot(weeklyPivots, price)
	} else {
		pivotWidth = "Normal"
	}

	// Compute S/R levels
	srLevels := CalcSRLevels(klines["1d"], klines["1w"], klines["1M"])
	if len(srLevels) > 0 {
		proximity = FindNearestSR(srLevels, price)
	}

	// Monthly S/R
	monthlySR := FindNearestMonthlySR(klines["1M"], price)

	// Swings
	swings1H := GetLatestSwing(klines["1h"])
	swings4H := GetLatestSwing(klines["4h"])
	swingsD := GetLatestSwing(klines["1d"])

	// --- Derivatives data ---

	// Open Interest (futures only)
	var oiChange OIChange
	if market == "futures" {
		currentOI, err := FetchOI(ctx, baseURL, symbol)
		if err != nil {
			e.logger.Debug("failed to fetch OI", slog.String("symbol", symbol), slog.Any("error", err))
		} else {
			oiHistory, err := FetchOIHistory(ctx, baseURL, symbol, "5m", 30)
			if err != nil {
				e.logger.Debug("failed to fetch OI history", slog.String("symbol", symbol), slog.Any("error", err))
			}
			oiChange = CalcOIChange(oiHistory, currentOI)
		}
		time.Sleep(50 * time.Millisecond) // rate limit delay
	}

	// Funding rate (futures only)
	var funding FundingData
	if market == "futures" {
		fd, err := FetchFundingRate(ctx, baseURL, symbol)
		if err != nil {
			e.logger.Debug("failed to fetch funding", slog.String("symbol", symbol), slog.Any("error", err))
		} else {
			funding = fd
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Liquidation clusters from 1h candles
	var liqEstimate LiqEstimate
	if candles1h, ok := klines["1h"]; ok && len(candles1h) > 0 {
		liqEstimate = EstimateLiqClusters(price, candles1h)
	}

	// Volume anomaly from 1h candles
	var volAnomaly VolumeAnomaly
	if candles1h, ok := klines["1h"]; ok && len(candles1h) > 0 {
		volAnomaly = DetectVolumeAnomaly(candles1h)
	}

	// Whale detection (futures only)
	var whaleSummary WhaleSummary
	if market == "futures" {
		trades, err := FetchRecentTrades(ctx, baseURL, symbol, 500)
		if err != nil {
			e.logger.Debug("failed to fetch trades", slog.String("symbol", symbol), slog.Any("error", err))
		} else {
			whales := DetectWhales(trades, DefaultWhaleThreshold)
			whaleSummary = SummarizeWhales(whales)
		}
		time.Sleep(50 * time.Millisecond)
	}

	return &CoinScan{
		Symbol:      symbol,
		Price:       price,
		Change24h:   change24h,
		PivotWidth:  pivotWidth,
		Bias1H:      bias1H,
		Bias4H:      bias4H,
		BiasD:       biasD,
		BiasW:       biasW,
		RSIValues:   rsiValues,
		RSIState:    rsiState,
		RSIHistory:  rsiHistory,
		Proximity:   proximity,
		NextFVG:     nextFVG,
		MonthlySR:   monthlySR,
		Swings1H:    swings1H,
		Swings4H:    swings4H,
		SwingsD:     swingsD,
		OIChange:    oiChange,
		Funding:     funding,
		LiqEstimate: liqEstimate,
		VolAnomaly:  volAnomaly,
		Whales:      whaleSummary,
		UpdatedAt:   time.Now(),
	}
}

// GetResults returns all current scan results.
func (e *ScannerEngine) GetResults() []CoinScan {
	e.mu.RLock()
	defer e.mu.RUnlock()

	out := make([]CoinScan, 0, len(e.results))
	for _, v := range e.results {
		out = append(out, *v)
	}
	return out
}

// GetConfig returns the current scanner config.
func (e *ScannerEngine) GetConfig() ScannerConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.config
}

// UpdateConfig updates the scanner config and saves to disk.
func (e *ScannerEngine) UpdateConfig(cfg ScannerConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if cfg.Exchange != "" {
		e.config.Exchange = cfg.Exchange
	}
	if cfg.Market != "" {
		e.config.Market = cfg.Market
	}
	if len(cfg.Symbols) > 0 {
		e.config.Symbols = cfg.Symbols
	}
	if cfg.ScanInterval > 0 {
		e.config.ScanInterval = cfg.ScanInterval
	}

	e.saveConfig()
}

// GetWatchlist returns the current watchlist.
func (e *ScannerEngine) GetWatchlist() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]string, len(e.watchlist))
	copy(out, e.watchlist)
	return out
}

// SetWatchlist updates the watchlist and saves to disk.
func (e *ScannerEngine) SetWatchlist(symbols []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.watchlist = symbols
	e.saveWatchlist()
}

func (e *ScannerEngine) saveConfig() {
	data, err := json.MarshalIndent(e.config, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(e.configPath, data, 0644)
}

func (e *ScannerEngine) saveWatchlist() {
	data, err := json.MarshalIndent(e.watchlist, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(e.watchPath, data, 0644)
}

func loadConfigFromDisk(configDir string) ScannerConfig {
	path := filepath.Join(configDir, "scanner.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultConfig()
	}

	var cfg ScannerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig()
	}
	if len(cfg.Symbols) == 0 {
		cfg.Symbols = DefaultSymbols()
	}
	if cfg.ScanInterval == 0 {
		cfg.ScanInterval = 60 * time.Second
	}
	if cfg.Market == "" {
		cfg.Market = "futures"
	}
	if cfg.Exchange == "" {
		cfg.Exchange = "binance"
	}
	return cfg
}

func loadWatchlistFromDisk(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var list []string
	_ = json.Unmarshal(data, &list)
	return list
}

// GetDerivativesData returns detailed derivatives data for a single symbol.
func (e *ScannerEngine) GetDerivativesData(symbol string) *CoinScan {
	e.mu.RLock()
	defer e.mu.RUnlock()

	scan, ok := e.results[symbol]
	if !ok {
		return nil
	}
	copy := *scan
	return &copy
}

// GetScatterData returns scatter plot data points.
func (e *ScannerEngine) GetScatterData() []ScatterPoint {
	e.mu.RLock()
	defer e.mu.RUnlock()

	points := make([]ScatterPoint, 0, len(e.results))
	for _, scan := range e.results {
		rsi := scan.RSIValues["1h"]
		points = append(points, ScatterPoint{
			Symbol:       scan.Symbol,
			RSI:          rsi,
			FVGProximity: scan.NextFVG.Proximity,
			RSIState:     scan.RSIState,
			FundingState: scan.Funding.State,
			OIChange4H:   scan.OIChange.Change4H,
			OIState:      scan.OIChange.State,
		})
	}
	return points
}
