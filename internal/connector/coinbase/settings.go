// Package coinbase implements a VisualHFT market data connector for the
// Coinbase Advanced Trade exchange. It uses gorilla/websocket for streaming
// market data and net/http for REST order book snapshots.
package coinbase

// Settings holds the configuration for the Coinbase connector.
type Settings struct {
	ApiKey       string   `json:"apiKey"`
	ApiSecret    string   `json:"apiSecret"`
	Symbols      []string `json:"symbols"`      // e.g. ["BTC-USD(BTC/USD)"]
	DepthLevels  int      `json:"depthLevels"`  // number of order book levels (default 25)
	ProviderID   int      `json:"providerId"`   // 7
	ProviderName string   `json:"providerName"` // "Coinbase"
}

// DefaultSettings returns a Settings populated with sensible defaults.
func DefaultSettings() Settings {
	return Settings{
		DepthLevels:  25,
		ProviderID:   7,
		ProviderName: "Coinbase",
	}
}
