package handlers

import (
	"net/http"

	"github.com/ashark-ai-05/tradefox/internal/core/models"
)

// GetProviders returns all known providers.
//
//	GET /api/providers
func GetProviders(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var providers []models.Provider

		deps.DataStore.providers.Range(func(_, value any) bool {
			providers = append(providers, value.(models.Provider))
			return true
		})

		if providers == nil {
			providers = []models.Provider{}
		}

		writeJSON(w, http.StatusOK, providers)
	}
}
