package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// GetTrades returns the recent trades for the given symbol.
//
//	GET /api/trades/{symbol}
func GetTrades(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		symbol := chi.URLParam(r, "symbol")
		if symbol == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "missing symbol parameter"})
			return
		}

		val, ok := deps.DataStore.trades.Load(symbol)
		if !ok {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "no trades found for symbol: " + symbol})
			return
		}

		buf := val.(*TradeBuffer)
		trades := buf.GetAll()
		writeJSON(w, http.StatusOK, trades)
	}
}
