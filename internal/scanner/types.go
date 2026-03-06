package scanner

import "time"

// ScannerConfig holds configuration for the multi-coin scanner.
type ScannerConfig struct {
	Exchange     string        `json:"exchange"`      // binance, bybit, etc
	Market       string        `json:"market"`        // futures or spot
	Symbols      []string      `json:"symbols"`       // configurable coin list
	ScanInterval time.Duration `json:"scan_interval"` // how often to scan
}

// CoinScan holds all computed indicators for a single coin.
type CoinScan struct {
	Symbol     string            `json:"symbol"`
	Price      float64           `json:"price"`
	Change24h  float64           `json:"change_24h"`
	PivotWidth string            `json:"pivot_width"` // Wide, Narrow, Normal
	Bias1H     BiasResult        `json:"bias_1h"`
	Bias4H     BiasResult        `json:"bias_4h"`
	BiasD      BiasResult        `json:"bias_d"`
	BiasW      BiasResult        `json:"bias_w"`
	RSIValues  map[string]float64   `json:"rsi_values"`  // keyed by timeframe
	RSIState   string               `json:"rsi_state"`   // StrongOversold..StrongOverbought
	RSIHistory map[string][]float64 `json:"rsi_history"` // last 20 values per TF
	Proximity  ProximityResult      `json:"proximity"`
	NextFVG    FVGResult            `json:"next_fvg"`
	MonthlySR  SRResult             `json:"monthly_sr"`
	Swings1H   SwingResult          `json:"swings_1h"`
	Swings4H   SwingResult          `json:"swings_4h"`
	SwingsD     SwingResult          `json:"swings_d"`
	OIChange    OIChange             `json:"oi_change"`
	Funding     FundingData          `json:"funding"`
	LiqEstimate LiqEstimate          `json:"liq_estimate"`
	VolAnomaly  VolumeAnomaly        `json:"vol_anomaly"`
	Whales      WhaleSummary         `json:"whales"`
	UpdatedAt   time.Time            `json:"updated_at"`
}

// BiasResult represents market bias for a timeframe.
type BiasResult struct {
	Direction string `json:"direction"` // High, Low, None
	Tag       string `json:"tag"`       // R (reversal), C (continuation)
}

// RSIResult holds RSI data for a single timeframe.
type RSIResult struct {
	Value   float64   `json:"value"`
	State   string    `json:"state"`
	History []float64 `json:"history"`
}

// FVGResult represents the nearest Fair Value Gap.
type FVGResult struct {
	Proximity float64 `json:"proximity"` // % distance to nearest FVG
	Type      string  `json:"type"`      // Bullish, Bearish
	Timeframe string  `json:"timeframe"` // Daily, Weekly, etc
	Level     float64 `json:"level"`     // price level
	FillPct   float64 `json:"fill_pct"`  // 50%, 100% etc
}

// ProximityResult represents distance to the nearest key level.
type ProximityResult struct {
	Distance float64 `json:"distance"` // % distance
	Type     string  `json:"type"`     // Support, Resistance
	Level    string  `json:"level"`    // PWH, PWL, PDH, PDL, etc
	Price    float64 `json:"price"`
}

// SRResult represents the nearest monthly support/resistance level.
type SRResult struct {
	Distance float64 `json:"distance"`
	Type     string  `json:"type"`  // Support, Resistance
	Level    string  `json:"level"` // Monthly R1, S1, etc
	Price    float64 `json:"price"`
}

// SwingResult represents the latest swing point.
type SwingResult struct {
	Type       string  `json:"type"`        // SwingHigh, SwingLow
	Class      string  `json:"class"`       // C1, C2, C3
	Price      float64 `json:"price"`
	CandlesAgo int     `json:"candles_ago"`
}

// Candle represents a single OHLCV candlestick.
type Candle struct {
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
	Volume    float64 `json:"volume"`
	OpenTime  int64   `json:"open_time"`
	CloseTime int64   `json:"close_time"`
}

// FVG represents a detected Fair Value Gap.
type FVG struct {
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Type      string  `json:"type"` // Bullish, Bearish
	Filled    bool    `json:"filled"`
	FillPct   float64 `json:"fill_pct"`
	Index     int     `json:"index"`
	Timeframe string  `json:"timeframe"`
}

// PivotLevels holds standard pivot point levels.
type PivotLevels struct {
	P  float64 `json:"p"`
	S1 float64 `json:"s1"`
	S2 float64 `json:"s2"`
	S3 float64 `json:"s3"`
	R1 float64 `json:"r1"`
	R2 float64 `json:"r2"`
	R3 float64 `json:"r3"`
}

// SRLevel represents a support or resistance level.
type SRLevel struct {
	Price float64 `json:"price"`
	Type  string  `json:"type"`  // Support, Resistance
	Label string  `json:"label"` // PWH, PWL, PDH, PDL, Monthly R1, etc
}

// SwingPoint represents a detected swing high or low.
type SwingPoint struct {
	Type       string  `json:"type"`  // SwingHigh, SwingLow
	Class      string  `json:"class"` // C1, C2, C3
	Price      float64 `json:"price"`
	Index      int     `json:"index"`
	CandlesAgo int     `json:"candles_ago"`
}

// ScatterPoint is used for the scatter plot API.
type ScatterPoint struct {
	Symbol       string  `json:"symbol"`
	RSI          float64 `json:"rsi"`
	FVGProximity float64 `json:"fvg_proximity"`
	RSIState     string  `json:"rsi_state"`
	FundingState string  `json:"funding_state"`
	OIChange4H   float64 `json:"oi_change_4h"`
	OIState      string  `json:"oi_state"`
}

// DefaultSymbols returns the default list of top 40 crypto symbols.
func DefaultSymbols() []string {
	return []string{
		"BTCUSDT", "ETHUSDT", "BNBUSDT", "SOLUSDT", "XRPUSDT",
		"DOGEUSDT", "ADAUSDT", "AVAXUSDT", "DOTUSDT", "LINKUSDT",
		"MATICUSDT", "UNIUSDT", "LTCUSDT", "ATOMUSDT", "NEARUSDT",
		"APTUSDT", "OPUSDT", "ARBUSDT", "FILUSDT", "INJUSDT",
		"SUIUSDT", "SEIUSDT", "TIAUSDT", "JUPUSDT", "WLDUSDT",
		"STXUSDT", "IMXUSDT", "RUNEUSDT", "FETUSDT", "AGIXUSDT",
		"RENDERUSDT", "PEOPLEUSDT", "FLOKIUSDT", "PEPEUSDT", "WIFUSDT",
		"BONKUSDT", "ORDIUSDT", "KASUSDT", "THETAUSDT", "GALAUSDT",
	}
}

// DefaultConfig returns the default scanner configuration.
func DefaultConfig() ScannerConfig {
	return ScannerConfig{
		Exchange:     "binance",
		Market:       "futures",
		Symbols:      DefaultSymbols(),
		ScanInterval: 60 * time.Second,
	}
}

// OIPoint represents a single open interest data point.
type OIPoint struct {
	OI        float64 `json:"oi"`
	Timestamp int64   `json:"timestamp"`
}

// OIChange represents open interest change across timeframes.
type OIChange struct {
	Change1H  float64 `json:"change_1h"`
	Change4H  float64 `json:"change_4h"`
	Change24H float64 `json:"change_24h"`
	RawOI     float64 `json:"raw_oi"`
	State     string  `json:"state"` // Building, Declining, Stable
}

// FundingData represents funding rate information.
type FundingData struct {
	Rate      float64 `json:"rate"`
	Predicted float64 `json:"predicted"`
	NextTime  int64   `json:"next_time"`
	State     string  `json:"state"` // Normal, Elevated, High, Extreme, NegElevated, NegHigh, NegExtreme
}

// LiqCluster represents an estimated liquidation cluster at a price level.
type LiqCluster struct {
	Price     float64 `json:"price"`
	EstVolume float64 `json:"est_volume"`
	Distance  float64 `json:"distance"` // % distance from current price
}

// LiqEstimate represents estimated liquidation clusters above and below price.
type LiqEstimate struct {
	AbovePrice   float64    `json:"above_price"`
	BelowPrice   float64    `json:"below_price"`
	NearestAbove LiqCluster `json:"nearest_above"`
	NearestBelow LiqCluster `json:"nearest_below"`
	Asymmetry    float64    `json:"asymmetry"` // ratio of above/below volume
}

// VolumeAnomaly represents volume anomaly detection results.
type VolumeAnomaly struct {
	CurrentVol float64 `json:"current_vol"`
	AvgVol     float64 `json:"avg_vol"`
	Ratio      float64 `json:"ratio"`
	State      string  `json:"state"` // Normal, Elevated, Unusual, Spike
}

// AggTrade represents an aggregated trade from the exchange.
type AggTrade struct {
	Price        float64 `json:"price"`
	Qty          float64 `json:"qty"`
	IsBuyerMaker bool    `json:"is_buyer_maker"`
	Time         int64   `json:"time"`
}

// WhaleTrade represents a single detected whale trade.
type WhaleTrade struct {
	Price    float64 `json:"price"`
	Size     float64 `json:"size"`
	Notional float64 `json:"notional"`
	Side     string  `json:"side"` // Buy, Sell
	Time     int64   `json:"time"`
}

// WhaleSummary summarizes whale activity for a symbol.
type WhaleSummary struct {
	Count         int     `json:"count"`
	TotalNotional float64 `json:"total_notional"`
	NetSide       string  `json:"net_side"` // Buy, Sell, Neutral
	LargestTrade  float64 `json:"largest_trade"`
	LastSeen      int64   `json:"last_seen"`
}

// Timeframes used by the scanner.
var (
	RSITimeframes  = []string{"5m", "15m", "1h", "4h", "12h", "1d"}
	BiasTimeframes = []string{"1h", "4h", "1d", "1w"}
	FVGTimeframes  = []string{"4h", "1d"}
)
