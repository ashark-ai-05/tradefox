package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
)

// orderBookResponse is the JSON-friendly representation of an OrderBook.
// The models.OrderBook keeps bids/asks unexported, so we project the data
// into this response struct.
type orderBookResponse struct {
	Symbol             string              `json:"symbol"`
	MaxDepth           int                 `json:"maxDepth"`
	PriceDecimalPlaces int                 `json:"priceDecimalPlaces"`
	SizeDecimalPlaces  int                 `json:"sizeDecimalPlaces"`
	SymbolMultiplier   float64             `json:"symbolMultiplier"`
	ProviderID         int                 `json:"providerId"`
	ProviderName       string              `json:"providerName"`
	Sequence           int64               `json:"sequence"`
	MidPrice           float64             `json:"midPrice"`
	Spread             float64             `json:"spread"`
	Bids               []models.BookItem   `json:"bids"`
	Asks               []models.BookItem   `json:"asks"`
}

// toOrderBookResponse converts an OrderBook to the JSON response struct.
func toOrderBookResponse(ob *models.OrderBook) orderBookResponse {
	return orderBookResponse{
		Symbol:             ob.Symbol,
		MaxDepth:           ob.MaxDepth,
		PriceDecimalPlaces: ob.PriceDecimalPlaces,
		SizeDecimalPlaces:  ob.SizeDecimalPlaces,
		SymbolMultiplier:   ob.SymbolMultiplier,
		ProviderID:         ob.ProviderID,
		ProviderName:       ob.ProviderName,
		Sequence:           ob.Sequence,
		MidPrice:           ob.MidPrice(),
		Spread:             ob.Spread(),
		Bids:               ob.Bids(),
		Asks:               ob.Asks(),
	}
}

// GetOrderBook returns the current order book for the given symbol.
//
//	GET /api/orderbook/{symbol}
func GetOrderBook(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		symbol := chi.URLParam(r, "symbol")
		if symbol == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "missing symbol parameter"})
			return
		}

		val, ok := deps.DataStore.orderbooks.Load(symbol)
		if !ok {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "order book not found for symbol: " + symbol})
			return
		}

		ob := val.(*models.OrderBook)
		writeJSON(w, http.StatusOK, toOrderBookResponse(ob))
	}
}
