// Package websocket implements a generic WebSocket connector for VisualHFT.
// It connects to a configurable WebSocket URL and receives pre-built JSON
// objects. No order book management, no delta merging, no authentication.
package websocket

// Settings holds the configuration for the WebSocket connector.
type Settings struct {
	HostName     string `json:"hostName"`     // WebSocket server hostname
	Port         int    `json:"port"`         // WebSocket server port
	ProviderID   int    `json:"providerId"`   // unique provider identifier
	ProviderName string `json:"providerName"` // human-readable provider name
}

// DefaultSettings returns Settings with sensible defaults.
func DefaultSettings() Settings {
	return Settings{
		HostName:     "localhost",
		Port:         6900,
		ProviderID:   3,
		ProviderName: "WebSocket",
	}
}
