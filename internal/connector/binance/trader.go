package binance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/core/enums"
	"github.com/ashark-ai-05/tradefox/internal/core/interfaces"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
)

const futuresBaseURL = "https://fapi.binance.com"

// FuturesTrader implements interfaces.Trader for Binance Futures.
type FuturesTrader struct {
	apiKey    string
	apiSecret string
	client    *http.Client
	logger    *slog.Logger
}

// FuturesTraderConfig holds the configuration for FuturesTrader.
type FuturesTraderConfig struct {
	APIKey    string `json:"apiKey"`
	APISecret string `json:"apiSecret"`
}

// NewFuturesTrader creates a new Binance Futures trader.
func NewFuturesTrader(cfg FuturesTraderConfig, logger *slog.Logger) *FuturesTrader {
	return &FuturesTrader{
		apiKey:    cfg.APIKey,
		apiSecret: cfg.APISecret,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// PlaceOrder places an order on Binance Futures via POST /fapi/v1/order.
func (ft *FuturesTrader) PlaceOrder(ctx context.Context, order *models.Order) (*models.Order, error) {
	params := url.Values{}
	params.Set("symbol", strings.ReplaceAll(order.Symbol, "/", ""))
	params.Set("side", mapSide(order.Side))
	params.Set("type", mapOrderType(order.OrderType))
	params.Set("quantity", strconv.FormatFloat(order.Quantity, 'f', -1, 64))

	if order.OrderType == enums.OrderTypeLimit || order.OrderType == enums.OrderTypeStopLimit {
		params.Set("price", strconv.FormatFloat(order.PricePlaced, 'f', -1, 64))
		params.Set("timeInForce", mapTimeInForce(order.TimeInForce))
	}

	if order.OrderType == enums.OrderTypeStopLimit {
		params.Set("stopPrice", strconv.FormatFloat(order.StopPrice, 'f', -1, 64))
	}

	body, err := ft.signedRequest(ctx, http.MethodPost, "/fapi/v1/order", params)
	if err != nil {
		return nil, err
	}

	var resp binanceOrderResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse order response: %w", err)
	}

	return resp.toOrder(), nil
}

// CancelOrder cancels an order via DELETE /fapi/v1/order.
func (ft *FuturesTrader) CancelOrder(ctx context.Context, symbol string, orderID int64) error {
	params := url.Values{}
	params.Set("symbol", strings.ReplaceAll(symbol, "/", ""))
	params.Set("orderId", strconv.FormatInt(orderID, 10))

	_, err := ft.signedRequest(ctx, http.MethodDelete, "/fapi/v1/order", params)
	return err
}

// ModifyOrder modifies an existing order by cancelling and replacing.
func (ft *FuturesTrader) ModifyOrder(ctx context.Context, symbol string, orderID int64, newQty, newPrice float64) (*models.Order, error) {
	params := url.Values{}
	params.Set("symbol", strings.ReplaceAll(symbol, "/", ""))
	params.Set("orderId", strconv.FormatInt(orderID, 10))
	params.Set("quantity", strconv.FormatFloat(newQty, 'f', -1, 64))
	params.Set("price", strconv.FormatFloat(newPrice, 'f', -1, 64))

	body, err := ft.signedRequest(ctx, http.MethodPut, "/fapi/v1/order", params)
	if err != nil {
		return nil, err
	}

	var resp binanceOrderResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse modify response: %w", err)
	}

	return resp.toOrder(), nil
}

// GetOpenOrders returns open orders via GET /fapi/v1/openOrders.
func (ft *FuturesTrader) GetOpenOrders(ctx context.Context, symbol string) ([]models.Order, error) {
	params := url.Values{}
	if symbol != "" {
		params.Set("symbol", strings.ReplaceAll(symbol, "/", ""))
	}

	body, err := ft.signedRequest(ctx, http.MethodGet, "/fapi/v1/openOrders", params)
	if err != nil {
		return nil, err
	}

	var resp []binanceOrderResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse open orders: %w", err)
	}

	orders := make([]models.Order, len(resp))
	for i, r := range resp {
		orders[i] = *r.toOrder()
	}
	return orders, nil
}

// GetOrderHistory returns recent order history via GET /fapi/v1/allOrders.
func (ft *FuturesTrader) GetOrderHistory(ctx context.Context, symbol string, limit int) ([]models.Order, error) {
	params := url.Values{}
	params.Set("symbol", strings.ReplaceAll(symbol, "/", ""))
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	body, err := ft.signedRequest(ctx, http.MethodGet, "/fapi/v1/allOrders", params)
	if err != nil {
		return nil, err
	}

	var resp []binanceOrderResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse order history: %w", err)
	}

	orders := make([]models.Order, len(resp))
	for i, r := range resp {
		orders[i] = *r.toOrder()
	}
	return orders, nil
}

// GetPositions returns positions via GET /fapi/v2/positionRisk.
func (ft *FuturesTrader) GetPositions(ctx context.Context) ([]interfaces.ExchangePosition, error) {
	params := url.Values{}

	body, err := ft.signedRequest(ctx, http.MethodGet, "/fapi/v2/positionRisk", params)
	if err != nil {
		return nil, err
	}

	var resp []binancePositionResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse positions: %w", err)
	}

	positions := make([]interfaces.ExchangePosition, 0, len(resp))
	for _, r := range resp {
		posAmt := parseFloat(r.PositionAmt)
		if posAmt == 0 {
			continue // Skip empty positions
		}
		positions = append(positions, interfaces.ExchangePosition{
			Symbol:           r.Symbol,
			Side:             determineSide(posAmt),
			PositionAmt:      posAmt,
			EntryPrice:       parseFloat(r.EntryPrice),
			MarkPrice:        parseFloat(r.MarkPrice),
			UnrealizedProfit: parseFloat(r.UnrealizedProfit),
			Leverage:         parseFloat(r.Leverage),
			Notional:         parseFloat(r.Notional),
			LiquidationPrice: parseFloat(r.LiquidationPrice),
		})
	}
	return positions, nil
}

// ClosePosition closes a position by placing a market order in the opposite direction.
func (ft *FuturesTrader) ClosePosition(ctx context.Context, symbol string, quantity float64) (*models.Order, error) {
	// Get current position to determine direction
	positions, err := ft.GetPositions(ctx)
	if err != nil {
		return nil, fmt.Errorf("get positions for close: %w", err)
	}

	cleanSymbol := strings.ReplaceAll(symbol, "/", "")
	var pos *interfaces.ExchangePosition
	for i, p := range positions {
		if p.Symbol == cleanSymbol {
			pos = &positions[i]
			break
		}
	}

	if pos == nil {
		return nil, fmt.Errorf("no open position for %s", symbol)
	}

	// Opposite side
	side := "SELL"
	closeQty := quantity
	if pos.PositionAmt < 0 {
		side = "BUY"
		if closeQty == 0 {
			closeQty = -pos.PositionAmt
		}
	} else if closeQty == 0 {
		closeQty = pos.PositionAmt
	}

	params := url.Values{}
	params.Set("symbol", cleanSymbol)
	params.Set("side", side)
	params.Set("type", "MARKET")
	params.Set("quantity", strconv.FormatFloat(closeQty, 'f', -1, 64))
	params.Set("reduceOnly", "true")

	body, err := ft.signedRequest(ctx, http.MethodPost, "/fapi/v1/order", params)
	if err != nil {
		return nil, err
	}

	var resp binanceOrderResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse close response: %w", err)
	}

	return resp.toOrder(), nil
}

// signedRequest makes an authenticated request to the Binance Futures API.
func (ft *FuturesTrader) signedRequest(ctx context.Context, method, path string, params url.Values) ([]byte, error) {
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	params.Set("recvWindow", "5000")

	// HMAC-SHA256 signature
	mac := hmac.New(sha256.New, []byte(ft.apiSecret))
	mac.Write([]byte(params.Encode()))
	signature := hex.EncodeToString(mac.Sum(nil))
	params.Set("signature", signature)

	fullURL := futuresBaseURL + path
	var req *http.Request
	var err error

	if method == http.MethodGet || method == http.MethodDelete {
		fullURL += "?" + params.Encode()
		req, err = http.NewRequestWithContext(ctx, method, fullURL, nil)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, fullURL, strings.NewReader(params.Encode()))
		if req != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	}
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("X-MBX-APIKEY", ft.apiKey)

	resp, err := ft.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("binance API error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// Binance response types

type binanceOrderResponse struct {
	OrderID       int64  `json:"orderId"`
	Symbol        string `json:"symbol"`
	ClientOrderID string `json:"clientOrderId"`
	Side          string `json:"side"`
	Type          string `json:"type"`
	Status        string `json:"status"`
	Price         string `json:"price"`
	OrigQty       string `json:"origQty"`
	ExecutedQty   string `json:"executedQty"`
	AvgPrice      string `json:"avgPrice"`
	StopPrice     string `json:"stopPrice"`
	TimeInForce   string `json:"timeInForce"`
	UpdateTime    int64  `json:"updateTime"`
}

func (r *binanceOrderResponse) toOrder() *models.Order {
	return &models.Order{
		OrderID:        r.OrderID,
		Symbol:         r.Symbol,
		ClOrdID:        r.ClientOrderID,
		Side:           parseSide(r.Side),
		OrderType:      parseOrderType(r.Type),
		Status:         parseStatus(r.Status),
		PricePlaced:    parseFloat(r.Price),
		Quantity:       parseFloat(r.OrigQty),
		FilledQuantity: parseFloat(r.ExecutedQty),
		FilledPrice:    parseFloat(r.AvgPrice),
		StopPrice:      parseFloat(r.StopPrice),
		Exchange:       "binance-futures",
		LastUpdated:    time.UnixMilli(r.UpdateTime),
	}
}

type binancePositionResponse struct {
	Symbol           string `json:"symbol"`
	PositionAmt      string `json:"positionAmt"`
	EntryPrice       string `json:"entryPrice"`
	MarkPrice        string `json:"markPrice"`
	UnrealizedProfit string `json:"unRealizedProfit"`
	Leverage         string `json:"leverage"`
	Notional         string `json:"notional"`
	LiquidationPrice string `json:"liquidationPrice"`
}

// Helpers

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func parseSide(s string) enums.OrderSide {
	switch s {
	case "BUY":
		return enums.OrderSideBuy
	case "SELL":
		return enums.OrderSideSell
	default:
		return enums.OrderSideNone
	}
}

func mapSide(s enums.OrderSide) string {
	switch s {
	case enums.OrderSideBuy:
		return "BUY"
	case enums.OrderSideSell:
		return "SELL"
	default:
		return "BUY"
	}
}

func parseOrderType(s string) enums.OrderType {
	switch s {
	case "LIMIT":
		return enums.OrderTypeLimit
	case "MARKET":
		return enums.OrderTypeMarket
	case "STOP":
		return enums.OrderTypeStopLimit
	default:
		return enums.OrderTypeNone
	}
}

func mapOrderType(t enums.OrderType) string {
	switch t {
	case enums.OrderTypeLimit:
		return "LIMIT"
	case enums.OrderTypeMarket:
		return "MARKET"
	case enums.OrderTypeStopLimit:
		return "STOP"
	default:
		return "MARKET"
	}
}

func mapTimeInForce(tif enums.OrderTimeInForce) string {
	switch tif {
	case enums.TimeInForceGTC:
		return "GTC"
	case enums.TimeInForceIOC:
		return "IOC"
	case enums.TimeInForceFOK:
		return "FOK"
	default:
		return "GTC"
	}
}

func parseStatus(s string) enums.OrderStatus {
	switch s {
	case "NEW":
		return enums.OrderStatusNew
	case "PARTIALLY_FILLED":
		return enums.OrderStatusPartialFilled
	case "FILLED":
		return enums.OrderStatusFilled
	case "CANCELED":
		return enums.OrderStatusCanceled
	case "REJECTED":
		return enums.OrderStatusRejected
	default:
		return enums.OrderStatusNone
	}
}

func determineSide(posAmt float64) string {
	if posAmt > 0 {
		return "LONG"
	}
	if posAmt < 0 {
		return "SHORT"
	}
	return "BOTH"
}
