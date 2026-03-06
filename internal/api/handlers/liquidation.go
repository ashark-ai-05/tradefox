package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/liquidation"
)

// LiquidationDeps holds dependencies for liquidation handlers.
type LiquidationDeps struct {
	Engine *liquidation.HeatmapEngine
}

// GetLiquidationHeatmap handles GET /api/liquidations/heatmap?symbol=BTCUSDT&range=5&bins=200.
func GetLiquidationHeatmap(ld *LiquidationDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		if symbol == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "symbol parameter required"})
			return
		}

		rangePercent := 5.0
		if v := r.URL.Query().Get("range"); v != "" {
			if parsed, err := strconv.ParseFloat(v, 64); err == nil && parsed > 0 {
				rangePercent = parsed
			}
		}

		numBins := 200
		if v := r.URL.Query().Get("bins"); v != "" {
			if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 1000 {
				numBins = parsed
			}
		}

		// Try cached heatmap first
		if cached := ld.Engine.Latest(symbol); cached != nil {
			writeJSON(w, http.StatusOK, cached)
			return
		}

		// Generate on demand if no cached version exists
		// Use 0 as price to indicate we need a price update first
		writeJSON(w, http.StatusOK, ld.Engine.GenerateHeatmap(symbol, 0, rangePercent, numBins))
	}
}

// GetLiquidationFeed handles GET /api/liquidations/feed?symbol=BTCUSDT&limit=100.
func GetLiquidationFeed(ld *LiquidationDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")

		limit := 100
		if v := r.URL.Query().Get("limit"); v != "" {
			if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 1000 {
				limit = parsed
			}
		}

		events := ld.Engine.Feed().RecentEvents(symbol, limit)
		if events == nil {
			events = []liquidation.LiquidationEvent{}
		}
		writeJSON(w, http.StatusOK, events)
	}
}

// GetLiquidationStats handles GET /api/liquidations/stats?symbol=BTCUSDT&period=1h.
func GetLiquidationStats(ld *LiquidationDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		if symbol == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "symbol parameter required"})
			return
		}

		period := 1 * time.Hour
		if v := r.URL.Query().Get("period"); v != "" {
			if parsed, err := time.ParseDuration(v); err == nil && parsed > 0 {
				period = parsed
			}
		}

		stats := ld.Engine.Feed().Stats(symbol, period)
		writeJSON(w, http.StatusOK, stats)
	}
}
