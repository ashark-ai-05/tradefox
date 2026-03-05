package models

import (
	"time"

	"github.com/ashark-ai-05/tradefox/internal/core/enums"
)

// Provider represents a market data provider and its connection status.
type Provider struct {
	ProviderID   int                 `json:"providerId"`
	ProviderCode int                 `json:"providerCode"`
	ProviderName string              `json:"providerName"`
	Status       enums.SessionStatus `json:"status"`
	LastUpdated  time.Time           `json:"lastUpdated"`
}

// Tooltip returns a human-readable description of the provider's connection status.
func (p *Provider) Tooltip() string {
	switch p.Status {
	case enums.SessionConnecting:
		return "Connecting..."
	case enums.SessionConnected:
		return "Connected"
	case enums.SessionConnectedWithWarnings:
		return "Connected with limitations"
	case enums.SessionDisconnectedFailed:
		return "Failure Disconnection"
	case enums.SessionDisconnected:
		return "Disconnected"
	default:
		return "Unknown"
	}
}
