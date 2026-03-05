package interfaces

import (
	"github.com/ashark-ai-05/tradefox/internal/core/enums"
)

// Plugin is the base interface for all VisualHFT plugins.
type Plugin interface {
	// Metadata
	Name() string
	Version() string
	Description() string
	Author() string
	PluginType() enums.PluginType
	PluginUniqueID() string
	RequiredLicenseLevel() enums.LicenseLevel

	// Lifecycle
	Status() enums.PluginStatus
	SetStatus(enums.PluginStatus)
}

// PluginInfo is a serializable snapshot of plugin state for API responses.
type PluginInfo struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Version     string             `json:"version"`
	Description string             `json:"description"`
	Author      string             `json:"author"`
	Type        enums.PluginType   `json:"type"`
	Status      enums.PluginStatus `json:"status"`
	License     enums.LicenseLevel `json:"licenseLevel"`
}
