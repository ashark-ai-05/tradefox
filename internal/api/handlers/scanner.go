package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/ashark-ai-05/tradefox/internal/scanner"
)

// ScannerDeps holds dependencies for scanner handlers.
type ScannerDeps struct {
	Engine *scanner.ScannerEngine
}

// GetScannerCoins handles GET /api/scanner/coins.
func GetScannerCoins(sd *ScannerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		results := sd.Engine.GetResults()
		writeJSON(w, http.StatusOK, results)
	}
}

// GetScannerScatter handles GET /api/scanner/scatter.
func GetScannerScatter(sd *ScannerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		points := sd.Engine.GetScatterData()
		writeJSON(w, http.StatusOK, points)
	}
}

// GetScannerSwings handles GET /api/scanner/swings.
func GetScannerSwings(sd *ScannerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		results := sd.Engine.GetResults()

		type swingEntry struct {
			Symbol string             `json:"symbol"`
			Swing  scanner.SwingResult `json:"swing"`
		}

		response := map[string][]swingEntry{
			"1h": {},
			"4h": {},
			"1d": {},
		}

		for _, scan := range results {
			if scan.Swings1H.Type != "" {
				response["1h"] = append(response["1h"], swingEntry{Symbol: scan.Symbol, Swing: scan.Swings1H})
			}
			if scan.Swings4H.Type != "" {
				response["4h"] = append(response["4h"], swingEntry{Symbol: scan.Symbol, Swing: scan.Swings4H})
			}
			if scan.SwingsD.Type != "" {
				response["1d"] = append(response["1d"], swingEntry{Symbol: scan.Symbol, Swing: scan.SwingsD})
			}
		}

		writeJSON(w, http.StatusOK, response)
	}
}

// GetScannerConfig handles GET /api/scanner/config.
func GetScannerConfig(sd *ScannerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := sd.Engine.GetConfig()
		writeJSON(w, http.StatusOK, cfg)
	}
}

// UpdateScannerConfig handles PUT /api/scanner/config.
func UpdateScannerConfig(sd *ScannerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var cfg scanner.ScannerConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
			return
		}
		sd.Engine.UpdateConfig(cfg)
		writeJSON(w, http.StatusOK, sd.Engine.GetConfig())
	}
}

// GetScannerWatchlist handles GET /api/scanner/watchlist.
func GetScannerWatchlist(sd *ScannerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list := sd.Engine.GetWatchlist()
		writeJSON(w, http.StatusOK, list)
	}
}

// GetScannerDerivatives handles GET /api/scanner/derivatives?symbol=X.
func GetScannerDerivatives(sd *ScannerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		if symbol == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "symbol parameter required"})
			return
		}

		data := sd.Engine.GetDerivativesData(symbol)
		if data == nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "symbol not found"})
			return
		}

		writeJSON(w, http.StatusOK, data)
	}
}

// UpdateScannerWatchlist handles PUT /api/scanner/watchlist.
func UpdateScannerWatchlist(sd *ScannerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var symbols []string
		if err := json.NewDecoder(r.Body).Decode(&symbols); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
			return
		}
		sd.Engine.SetWatchlist(symbols)
		writeJSON(w, http.StatusOK, symbols)
	}
}
