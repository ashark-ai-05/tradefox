package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/ashark-ai-05/tradefox/internal/core/enums"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/execution"
)

// ExecutionDeps holds dependencies for execution handlers.
type ExecutionDeps struct {
	Executor *execution.Executor
	Presets  *execution.PresetStore
}

// --- Order Handlers ---

type placeOrderRequest struct {
	Exchange    string  `json:"exchange"`
	Symbol      string  `json:"symbol"`
	Side        string  `json:"side"`        // "Buy" or "Sell"
	OrderType   string  `json:"orderType"`   // "Market", "Limit", "StopLimit"
	Quantity    float64 `json:"quantity"`
	Price       float64 `json:"price,omitempty"`
	StopPrice   float64 `json:"stopPrice,omitempty"`
	Leverage    float64 `json:"leverage,omitempty"`
	StopLoss    float64 `json:"stopLoss,omitempty"`
	TakeProfit  float64 `json:"takeProfit,omitempty"`
	TimeInForce string  `json:"timeInForce,omitempty"`
}

// PlaceOrder handles POST /api/orders.
func PlaceOrder(ed *ExecutionDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req placeOrderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
			return
		}

		order := &models.Order{
			Symbol:      req.Symbol,
			Side:        parseSideString(req.Side),
			OrderType:   parseOrderTypeString(req.OrderType),
			Quantity:    req.Quantity,
			PricePlaced: req.Price,
			StopPrice:   req.StopPrice,
			Leverage:    req.Leverage,
			StopLoss:    req.StopLoss,
			TakeProfit:  req.TakeProfit,
			TimeInForce: parseTimeInForceString(req.TimeInForce),
			Exchange:    req.Exchange,
		}

		result, err := ed.Executor.PlaceOrder(r.Context(), req.Exchange, order)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		writeJSON(w, http.StatusCreated, result)
	}
}

// CancelOrder handles DELETE /api/orders/{id}.
func CancelOrder(ed *ExecutionDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		orderID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid order ID"})
			return
		}

		exchange := r.URL.Query().Get("exchange")
		symbol := r.URL.Query().Get("symbol")
		if exchange == "" || symbol == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "exchange and symbol query params required"})
			return
		}

		if err := ed.Executor.CancelOrder(r.Context(), exchange, symbol, orderID); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
	}
}

// ModifyOrder handles PATCH /api/orders/{id}.
func ModifyOrder(ed *ExecutionDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		orderID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid order ID"})
			return
		}

		var req struct {
			Exchange string  `json:"exchange"`
			Symbol   string  `json:"symbol"`
			Quantity float64 `json:"quantity"`
			Price    float64 `json:"price"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
			return
		}

		result, err := ed.Executor.ModifyOrder(r.Context(), req.Exchange, req.Symbol, orderID, req.Quantity, req.Price)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, result)
	}
}

// GetOpenOrders handles GET /api/orders.
func GetOpenOrders(ed *ExecutionDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		exchange := r.URL.Query().Get("exchange")
		symbol := r.URL.Query().Get("symbol")
		if exchange == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "exchange query param required"})
			return
		}

		orders, err := ed.Executor.GetOpenOrders(r.Context(), exchange, symbol)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, orders)
	}
}

// GetOrderHistory handles GET /api/orders/history.
func GetOrderHistory(ed *ExecutionDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		exchange := r.URL.Query().Get("exchange")
		symbol := r.URL.Query().Get("symbol")
		if exchange == "" || symbol == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "exchange and symbol query params required"})
			return
		}

		limit := 50
		if l := r.URL.Query().Get("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil {
				limit = v
			}
		}

		orders, err := ed.Executor.GetOrderHistory(r.Context(), exchange, symbol, limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, orders)
	}
}

// --- Position Handlers ---

// GetExchangePositions handles GET /api/positions.
func GetExchangePositions(ed *ExecutionDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		exchange := r.URL.Query().Get("exchange")
		if exchange == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "exchange query param required"})
			return
		}

		positions, err := ed.Executor.GetPositions(r.Context(), exchange)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, positions)
	}
}

// ClosePosition handles POST /api/positions/close.
func ClosePosition(ed *ExecutionDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Exchange string  `json:"exchange"`
			Symbol   string  `json:"symbol"`
			Quantity float64 `json:"quantity"` // 0 = close entire position
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
			return
		}

		result, err := ed.Executor.ClosePosition(r.Context(), req.Exchange, req.Symbol, req.Quantity)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, result)
	}
}

// --- Preset Handlers ---

// SavePreset handles POST /api/orders/presets.
func SavePreset(ed *ExecutionDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var preset execution.OrderPreset
		if err := json.NewDecoder(r.Body).Decode(&preset); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
			return
		}

		if err := ed.Presets.Save(preset); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		writeJSON(w, http.StatusCreated, preset)
	}
}

// ListPresets handles GET /api/orders/presets.
func ListPresets(ed *ExecutionDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, ed.Presets.List())
	}
}

// DeletePreset handles DELETE /api/orders/presets/{name}.
func DeletePreset(ed *ExecutionDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		if err := ed.Presets.Delete(name); err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}

// --- Risk Handlers ---

// KillSwitch handles POST /api/killswitch.
func KillSwitch(ed *ExecutionDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Active bool `json:"active"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
			return
		}

		if req.Active {
			ed.Executor.Risk().ActivateKillSwitch()
		} else {
			ed.Executor.Risk().DeactivateKillSwitch()
		}

		writeJSON(w, http.StatusOK, ed.Executor.Risk().Status())
	}
}

// GetRiskStatus handles GET /api/risk/status.
func GetRiskStatus(ed *ExecutionDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, ed.Executor.Risk().Status())
	}
}

// --- Helpers ---

func parseSideString(s string) enums.OrderSide {
	switch s {
	case "Buy":
		return enums.OrderSideBuy
	case "Sell":
		return enums.OrderSideSell
	default:
		return enums.OrderSideNone
	}
}

func parseOrderTypeString(s string) enums.OrderType {
	switch s {
	case "Limit":
		return enums.OrderTypeLimit
	case "Market":
		return enums.OrderTypeMarket
	case "StopLimit":
		return enums.OrderTypeStopLimit
	default:
		return enums.OrderTypeMarket
	}
}

func parseTimeInForceString(s string) enums.OrderTimeInForce {
	switch s {
	case "GTC":
		return enums.TimeInForceGTC
	case "IOC":
		return enums.TimeInForceIOC
	case "FOK":
		return enums.TimeInForceFOK
	default:
		return enums.TimeInForceGTC
	}
}
