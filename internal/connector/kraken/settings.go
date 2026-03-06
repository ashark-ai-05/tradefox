// Package kraken implements a VisualHFT market data connector for the
// Kraken exchange. It uses gorilla/websocket for streaming order book and
// trade data via the Kraken WebSocket API v2.
package kraken

// Settings holds the configuration for the Kraken connector.
type Settings struct {
	ApiKey       string   `json:"apiKey"`
	ApiSecret    string   `json:"apiSecret"`
	Symbols      []string `json:"symbols"`      // e.g. ["BTC/USD"]
	DepthLevels  int      `json:"depthLevels"`  // number of order book levels (default 25)
	ProviderID   int      `json:"providerId"`   // 3
	ProviderName string   `json:"providerName"` // "Kraken"
}

// DefaultSettings returns a Settings populated with sensible defaults.
func DefaultSettings() Settings {
	return Settings{
		DepthLevels:  25,
		ProviderID:   3,
		ProviderName: "Kraken",
	}
}
