// Package gemini implements a VisualHFT market data connector for the Gemini
// exchange. It uses gorilla/websocket for streaming L2 market data and user
// order events over two separate WebSocket connections.
package gemini

// Settings holds the configuration for the Gemini connector.
type Settings struct {
	HostName                   string   `json:"hostName"`                   // e.g. "https://api.gemini.com/v1/book/"
	WebSocketHostName          string   `json:"webSocketHostName"`          // e.g. "wss://api.gemini.com/v2/marketdata"
	WebSocketHostNameUserOrder string   `json:"webSocketHostNameUserOrder"` // e.g. "wss://api.gemini.com/v1/order/events"
	ApiKey                     string   `json:"apiKey"`
	ApiSecret                  string   `json:"apiSecret"`
	Symbols                    []string `json:"symbols"`      // e.g. ["BTCUSD(BTC/USD)"]
	DepthLevels                int      `json:"depthLevels"`  // number of order book levels (default 20)
	ProviderID                 int      `json:"providerId"`   // unique provider identifier (default 5)
	ProviderName               string   `json:"providerName"` // human-readable provider name (default "Gemini")
}

// DefaultSettings returns a Settings populated with sensible defaults for the
// Gemini exchange.
func DefaultSettings() Settings {
	return Settings{
		HostName:                   "https://api.gemini.com/v1/book/",
		WebSocketHostName:          "wss://api.gemini.com/v2/marketdata",
		WebSocketHostNameUserOrder: "wss://api.gemini.com/v1/order/events",
		DepthLevels:                20,
		ProviderID:                 5,
		ProviderName:               "Gemini",
	}
}
