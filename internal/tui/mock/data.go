package mock

import "fmt"

func f(v float64) *float64 { return &v }

// WatchlistEntry represents a single row in the watchlist.
type WatchlistEntry struct {
	Symbol   string
	Price    float64
	Change24 float64
	Volume   float64
	Bid      float64
	Ask      float64
}

// Spread returns ask - bid.
func (w WatchlistEntry) Spread() float64 { return w.Ask - w.Bid }

// OrderBookLevel represents a single level in the order book.
type OrderBookLevel struct {
	Price float64
	Qty   float64
	IsBid bool
}

// PositionEntry represents an open position.
type PositionEntry struct {
	Symbol    string
	Side      string
	Size      float64
	Entry     float64
	Mark      float64
	LiqPrice  float64
	Leverage  float64
}

func (p PositionEntry) PnL() float64 {
	if p.Side == "LONG" {
		return (p.Mark - p.Entry) * p.Size
	}
	return (p.Entry - p.Mark) * p.Size
}

func (p PositionEntry) PnLPct() float64 {
	if p.Entry == 0 {
		return 0
	}
	if p.Side == "LONG" {
		return ((p.Mark - p.Entry) / p.Entry) * 100
	}
	return ((p.Entry - p.Mark) / p.Entry) * 100
}

// ScannerEntry represents one row in the scanner view.
type ScannerEntry struct {
	Symbol  string
	Price   float64
	ChgPct  float64
	RSI1H   float64
	Bias    string
	FVG     string
	OIChg   float64
	Funding float64
	Volume  float64
}

// Watchlist returns mock watchlist data.
func Watchlist() []WatchlistEntry {
	return []WatchlistEntry{
		{"BTCUSDT", 67432.50, 2.34, 28547123000, 67432.00, 67433.00},
		{"ETHUSDT", 3521.80, 1.87, 15234567000, 3521.50, 3522.10},
		{"SOLUSDT", 142.35, 5.21, 4523678000, 142.30, 142.40},
		{"BNBUSDT", 598.20, -0.45, 1234567000, 598.10, 598.30},
		{"XRPUSDT", 0.6234, 3.12, 2345678000, 0.6232, 0.6236},
		{"ADAUSDT", 0.4567, -1.23, 987654000, 0.4565, 0.4569},
		{"DOGEUSDT", 0.1234, 8.45, 3456789000, 0.1233, 0.1235},
		{"AVAXUSDT", 35.67, -2.15, 876543000, 35.65, 35.69},
		{"DOTUSDT", 7.89, 0.67, 543210000, 7.88, 7.90},
		{"MATICUSDT", 0.7845, 1.45, 654321000, 0.7843, 0.7847},
		{"LINKUSDT", 14.56, 3.78, 432109000, 14.55, 14.57},
		{"UNIUSDT", 9.87, -0.89, 321098000, 9.86, 9.88},
		{"ATOMUSDT", 8.92, 2.56, 234567000, 8.91, 8.93},
		{"LTCUSDT", 83.45, -1.67, 567890000, 83.43, 83.47},
		{"NEARUSDT", 5.67, 4.32, 345678000, 5.66, 5.68},
		{"ARUSDT", 28.34, 6.78, 123456000, 28.32, 28.36},
		{"APTUSDT", 8.45, -3.21, 234567000, 8.44, 8.46},
		{"SUIUSDT", 1.23, 12.34, 456789000, 1.22, 1.24},
		{"OPUSDT", 2.34, -0.56, 345678000, 2.33, 2.35},
		{"ARBUSDT", 1.12, 1.89, 234567000, 1.11, 1.13},
	}
}

// OrderBook returns mock order book data for BTC (15 levels each side).
func OrderBook() (bids []OrderBookLevel, asks []OrderBookLevel) {
	basePrice := 67432.50
	for i := 0; i < 15; i++ {
		bids = append(bids, OrderBookLevel{
			Price: basePrice - float64(i)*0.50,
			Qty:   0.5 + float64(i)*0.3 + float64((i*7)%5)*0.2,
			IsBid: true,
		})
		asks = append(asks, OrderBookLevel{
			Price: basePrice + 0.50 + float64(i)*0.50,
			Qty:   0.4 + float64(i)*0.25 + float64((i*11)%5)*0.15,
			IsBid: false,
		})
	}
	return bids, asks
}

// Positions returns mock open positions.
func Positions() []PositionEntry {
	return []PositionEntry{
		{"BTCUSDT", "LONG", 0.5, 66800.00, 67432.50, 59120.00, 10},
		{"ETHUSDT", "SHORT", 5.0, 3580.00, 3521.80, 3938.00, 10},
		{"SOLUSDT", "LONG", 50.0, 138.50, 142.35, 124.65, 10},
		{"DOGEUSDT", "SHORT", 10000.0, 0.1280, 0.1234, 0.1408, 10},
	}
}

// Scanner returns mock scanner data.
func Scanner() []ScannerEntry {
	symbols := []string{
		"BTCUSDT", "ETHUSDT", "SOLUSDT", "BNBUSDT", "XRPUSDT",
		"ADAUSDT", "DOGEUSDT", "AVAXUSDT", "DOTUSDT", "MATICUSDT",
		"LINKUSDT", "UNIUSDT", "ATOMUSDT", "LTCUSDT", "NEARUSDT",
		"ARUSDT", "APTUSDT", "SUIUSDT", "OPUSDT", "ARBUSDT",
		"FILUSDT", "INJUSDT", "TIAUSDT", "SEIUSDT", "JUPUSDT",
		"WIFUSDT", "PENDLEUSDT", "STXUSDT", "RNDRUSDT", "FETUSDT",
		"THETAUSDT", "ALGOUSDT", "FTMUSDT", "SANDUSDT", "MANAUSDT",
		"GALAUSDT", "AXSUSDT", "APEUSDT", "LRCUSDT", "ENSUSDT",
	}
	biases := []string{"Bullish", "Bearish", "Neutral", "Bullish", "Bearish"}
	fvgs := []string{"Bullish", "Bearish", "None", "Bullish", "None"}

	prices := []float64{
		67432, 3521, 142.3, 598, 0.623, 0.456, 0.123, 35.6, 7.89, 0.784,
		14.56, 9.87, 8.92, 83.4, 5.67, 28.3, 8.45, 1.23, 2.34, 1.12,
		5.67, 23.4, 8.90, 0.45, 0.89, 2.34, 5.67, 1.78, 7.89, 1.23,
		1.45, 0.23, 0.67, 0.45, 0.56, 0.034, 7.89, 1.67, 0.34, 18.90,
	}

	entries := make([]ScannerEntry, len(symbols))
	for i, sym := range symbols {
		chg := float64((i*17+3)%200-100) / 10.0
		rsi := 30.0 + float64((i*13+7)%40)
		oiChg := float64((i*19+5)%300-150) / 10.0
		funding := float64((i*7+2)%20-10) / 1000.0
		vol := float64((i*23+11)%500+100) * 1_000_000

		entries[i] = ScannerEntry{
			Symbol:  sym,
			Price:   prices[i%len(prices)],
			ChgPct:  chg,
			RSI1H:   rsi,
			Bias:    biases[i%len(biases)],
			FVG:     fvgs[i%len(fvgs)],
			OIChg:   oiChg,
			Funding: funding,
			Volume:  vol,
		}
	}
	return entries
}

// Candle represents a single OHLCV candlestick.
type Candle struct {
	Time   int64
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

// SignalValue represents a single microstructure signal reading.
type SignalValue struct {
	Name  string
	Value float64
	State string // "Bullish", "Bearish", "Neutral"
}

// SignalSet holds all 8 microstructure signals.
type SignalSet struct {
	Signals        []SignalValue
	CompositeScore float64
}

// Trade represents a single recent trade.
type Trade struct {
	Time  int64
	Price float64
	Size  float64
	Side  string // "BUY" or "SELL"
}

// LiqBand represents a liquidation cluster at a price level.
type LiqBand struct {
	Price     float64
	Density   float64 // 0.0 - 1.0
	Side      string  // "LONG" or "SHORT"
}

// GenerateMockCandles creates realistic random walk candle data.
func GenerateMockCandles(symbol string, timeframe string, count int) []Candle {
	basePrice := 67432.50
	switch symbol {
	case "ETHUSDT":
		basePrice = 3521.80
	case "SOLUSDT":
		basePrice = 142.35
	case "BNBUSDT":
		basePrice = 598.20
	}

	// Volatility per timeframe
	var volatility float64
	var intervalSec int64
	switch timeframe {
	case "1m":
		volatility = 0.0003
		intervalSec = 60
	case "5m":
		volatility = 0.0007
		intervalSec = 300
	case "15m":
		volatility = 0.0012
		intervalSec = 900
	case "1h":
		volatility = 0.0025
		intervalSec = 3600
	case "4h":
		volatility = 0.005
		intervalSec = 14400
	default: // 1D
		volatility = 0.012
		intervalSec = 86400
	}

	candles := make([]Candle, count)
	price := basePrice
	seed := uint64(len(symbol)*31 + len(timeframe)*17 + count*7)

	for i := 0; i < count; i++ {
		// Simple deterministic pseudo-random using xorshift
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		r1 := float64(seed%10000)/10000.0 - 0.5
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		r2 := float64(seed%10000)/10000.0 - 0.5
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		r3 := float64(seed%10000) / 10000.0

		open := price
		change := open * volatility * r1 * 2
		close := open + change
		wick1 := open * volatility * (0.5 + r3*0.5)
		wick2 := open * volatility * (0.3 + r2*0.3)

		var high, low float64
		if close > open {
			high = close + wick1
			low = open - wick2
		} else {
			high = open + wick1
			low = close - wick2
		}

		// Volume correlates with price movement magnitude
		baseVol := basePrice * 10
		volMultiplier := 1.0 + 3.0*r3
		if r1 > 0.3 || r1 < -0.3 {
			volMultiplier *= 2.0
		}

		candles[i] = Candle{
			Time:   int64(i) * intervalSec,
			Open:   open,
			High:   high,
			Low:    low,
			Close:  close,
			Volume: baseVol * volMultiplier,
		}

		price = close
	}
	return candles
}

// GenerateMockSignals returns 8 microstructure signal values.
func GenerateMockSignals() SignalSet {
	signals := []SignalValue{
		{"Microprice", 0.62, "Bullish"},
		{"OFI", -0.15, "Bearish"},
		{"Sweep", 0.78, "Bullish"},
		{"Depth Imbal", 0.34, "Bullish"},
		{"Kyle Lambda", -0.45, "Bearish"},
		{"Volatility", 0.55, "Neutral"},
		{"Spoof Detect", 0.12, "Neutral"},
		{"Composite", 0.38, "Bullish"},
	}
	return SignalSet{
		Signals:        signals,
		CompositeScore: 0.38,
	}
}

// GenerateMockTrades creates recent mock trades.
func GenerateMockTrades(count int) []Trade {
	trades := make([]Trade, count)
	basePrice := 67432.50
	seed := uint64(42)

	for i := 0; i < count; i++ {
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		r1 := float64(seed%10000)/10000.0 - 0.5

		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		r2 := float64(seed%10000) / 10000.0

		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		r3 := float64(seed%10000) / 10000.0

		price := basePrice + r1*20.0
		side := "BUY"
		if r2 < 0.48 {
			side = "SELL"
		}

		// Size: mostly small, occasionally large
		size := 0.001 + r3*0.1
		if r3 > 0.92 {
			size = 0.5 + r3*2.0 // large trade
		} else if r3 > 0.8 {
			size = 0.1 + r3*0.5
		}

		trades[i] = Trade{
			Time:  int64(count-i) * 3,
			Price: price,
			Size:  size,
			Side:  side,
		}
	}
	return trades
}

// GenerateMockLiquidationBands generates liquidation clusters around current price.
func GenerateMockLiquidationBands(currentPrice float64, count int) []LiqBand {
	bands := make([]LiqBand, count*2)
	step := currentPrice * 0.002 // 0.2% per level

	seed := uint64(12345)
	for i := 0; i < count; i++ {
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		densityLong := float64(seed%1000) / 1000.0

		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		densityShort := float64(seed%1000) / 1000.0

		// Long liquidations below price
		bands[i] = LiqBand{
			Price:   currentPrice - step*float64(i+1),
			Density: densityLong * 0.8,
			Side:    "LONG",
		}
		// Short liquidations above price
		bands[count+i] = LiqBand{
			Price:   currentPrice + step*float64(i+1),
			Density: densityShort * 0.8,
			Side:    "SHORT",
		}
	}
	return bands
}

// FormatVolume formats volume with K/M/B suffixes.
func FormatVolume(v float64) string {
	switch {
	case v >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", v/1_000_000_000)
	case v >= 1_000_000:
		return fmt.Sprintf("%.1fM", v/1_000_000)
	case v >= 1_000:
		return fmt.Sprintf("%.1fK", v/1_000)
	default:
		return fmt.Sprintf("%.0f", v)
	}
}
