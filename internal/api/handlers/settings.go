package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/ashark-ai-05/tradefox/internal/config"
)

// GetSettings returns the current server configuration settings.
//
//	GET /api/settings
func GetSettings(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Settings == nil {
			writeJSON(w, http.StatusOK, config.ServerConfig{})
			return
		}

		cfg := deps.Settings.GetServerConfig()
		writeJSON(w, http.StatusOK, cfg)
	}
}

// UpdateSettings updates the server configuration settings.
// Expects a JSON body matching config.ServerConfig.
//
//	PUT /api/settings
func UpdateSettings(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Settings == nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "settings manager not available"})
			return
		}

		var cfg config.ServerConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body: " + err.Error()})
			return
		}

		deps.Settings.SetServerConfig(cfg)

		if err := deps.Settings.Save(); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to save settings: " + err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, cfg)
	}
}
