// Package kucoin implements a VisualHFT market data connector for the
// KuCoin exchange. It uses gorilla/websocket for streaming order book and
// trade data via the KuCoin WebSocket API.
package kucoin

// Settings holds the configuration for the KuCoin connector.
type Settings struct {
	ApiKey       string   `json:"apiKey"`
	ApiSecret    string   `json:"apiSecret"`
	Passphrase   string   `json:"passphrase"`
	Symbols      []string `json:"symbols"`      // e.g. ["BTC-USDT(BTC/USDT)"]
	DepthLevels  int      `json:"depthLevels"`  // number of order book levels (default 25)
	ProviderID   int      `json:"providerId"`   // 4
	ProviderName string   `json:"providerName"` // "KuCoin"
	// WSURL is the WebSocket endpoint (including token). For production use,
	// obtain from POST https://api.kucoin.com/api/v1/bullet-public.
	WSURL string `json:"wsUrl"`
	// RestBaseURL is the REST API base URL for fetching order book snapshots.
	RestBaseURL string `json:"restBaseUrl"`
}

// DefaultSettings returns a Settings populated with sensible defaults.
func DefaultSettings() Settings {
	return Settings{
		DepthLevels:  25,
		ProviderID:   4,
		ProviderName: "KuCoin",
		RestBaseURL:  "https://api.kucoin.com",
	}
}
