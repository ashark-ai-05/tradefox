package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// ListPlugins returns information about all registered plugins.
//
//	GET /api/plugins
func ListPlugins(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.PluginMgr == nil {
			writeJSON(w, http.StatusOK, []any{})
			return
		}

		infos := deps.PluginMgr.ListPlugins()
		writeJSON(w, http.StatusOK, infos)
	}
}

// StartPlugin starts a plugin by its unique ID.
//
//	POST /api/plugins/{id}/start
func StartPlugin(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "missing plugin id"})
			return
		}

		if deps.PluginMgr == nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "plugin manager not available"})
			return
		}

		if err := deps.PluginMgr.StartPlugin(r.Context(), id); err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "started", "id": id})
	}
}

// StopPlugin stops a plugin by its unique ID.
//
//	POST /api/plugins/{id}/stop
func StopPlugin(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "missing plugin id"})
			return
		}

		if deps.PluginMgr == nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "plugin manager not available"})
			return
		}

		if err := deps.PluginMgr.StopPlugin(r.Context(), id); err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "stopped", "id": id})
	}
}
