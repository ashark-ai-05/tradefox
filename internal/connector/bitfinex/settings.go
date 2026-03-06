// Package bitfinex implements a VisualHFT market data connector for the
// Bitfinex exchange. It uses gorilla/websocket for streaming order book and
// trade data via the Bitfinex WebSocket API v2.
package bitfinex

// Settings holds the configuration for the Bitfinex connector.
type Settings struct {
	ApiKey       string   `json:"apiKey"`
	ApiSecret    string   `json:"apiSecret"`
	Symbols      []string `json:"symbols"`      // e.g. ["BTCUSD(BTC/USD)"]
	DepthLevels  int      `json:"depthLevels"`  // number of order book levels (default 25)
	ProviderID   int      `json:"providerId"`   // 2
	ProviderName string   `json:"providerName"` // "Bitfinex"
}

// DefaultSettings returns a Settings populated with sensible defaults.
func DefaultSettings() Settings {
	return Settings{
		DepthLevels:  25,
		ProviderID:   2,
		ProviderName: "Bitfinex",
	}
}
