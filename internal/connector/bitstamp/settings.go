// Package bitstamp implements the BitStamp exchange connector for VisualHFT.
package bitstamp

// Settings holds the configuration for the BitStamp connector.
type Settings struct {
	HostName          string   `json:"hostName"`          // e.g. "https://www.bitstamp.net/api/v2/"
	WebSocketHostName string   `json:"webSocketHostName"` // e.g. "wss://ws.bitstamp.net"
	ApiKey            string   `json:"apiKey"`
	ApiSecret         string   `json:"apiSecret"`
	Symbols           []string `json:"symbols"`      // e.g. ["btcusd(BTC/USD)"]
	DepthLevels       int      `json:"depthLevels"`  // default 10
	ProviderID        int      `json:"providerId"`   // 6
	ProviderName      string   `json:"providerName"` // "BitStamp"
}

// DefaultSettings returns a Settings with sensible defaults for BitStamp.
func DefaultSettings() Settings {
	return Settings{
		HostName:          "https://www.bitstamp.net/api/v2/",
		WebSocketHostName: "wss://ws.bitstamp.net",
		Symbols:           []string{"btcusd(BTC/USD)"},
		DepthLevels:       10,
		ProviderID:        6,
		ProviderName:      "BitStamp",
	}
}
