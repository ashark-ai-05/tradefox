// Package binance implements a VisualHFT market data connector for the
// Binance exchange. It uses gorilla/websocket for streaming market data and
// net/http for REST order book snapshots.
package binance

// Settings holds the configuration for the Binance connector.
type Settings struct {
	ApiKey           string   `json:"apiKey"`
	ApiSecret        string   `json:"apiSecret"`
	Symbols          []string `json:"symbols"`          // e.g. ["BTCUSDT(BTC/USDT)", "ETHUSDT"]
	DepthLevels      int      `json:"depthLevels"`      // number of order book levels (default 10)
	UpdateIntervalMs int      `json:"updateIntervalMs"` // depth update interval in ms (default 100)
	IsNonUS          bool     `json:"isNonUS"`          // true = global (binance.com), false = US (binance.us)
	ProviderID       int      `json:"providerId"`
	ProviderName     string   `json:"providerName"`
}

// DefaultSettings returns a Settings populated with sensible defaults.
func DefaultSettings() Settings {
	return Settings{
		DepthLevels:      10,
		UpdateIntervalMs: 100,
		IsNonUS:          true,
		ProviderID:       1,
		ProviderName:     "Binance",
	}
}
