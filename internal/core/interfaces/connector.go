package interfaces

import (
	"context"

	"github.com/ashark-ai-05/tradefox/internal/core/models"
)

// Connector represents a market data source (exchange connector).
type Connector interface {
	Plugin
	StartAsync(ctx context.Context) error
	StopAsync(ctx context.Context) error
}

// Trader represents an exchange connector capable of order execution.
type Trader interface {
	PlaceOrder(ctx context.Context, order *models.Order) (*models.Order, error)
	CancelOrder(ctx context.Context, symbol string, orderID int64) error
	ModifyOrder(ctx context.Context, symbol string, orderID int64, newQty, newPrice float64) (*models.Order, error)
	GetOpenOrders(ctx context.Context, symbol string) ([]models.Order, error)
	GetOrderHistory(ctx context.Context, symbol string, limit int) ([]models.Order, error)
	GetPositions(ctx context.Context) ([]ExchangePosition, error)
	ClosePosition(ctx context.Context, symbol string, quantity float64) (*models.Order, error)
}

// ExchangePosition represents a position as reported by the exchange.
type ExchangePosition struct {
	Symbol           string  `json:"symbol"`
	Side             string  `json:"side"`           // "LONG", "SHORT", "BOTH"
	PositionAmt      float64 `json:"positionAmt"`
	EntryPrice       float64 `json:"entryPrice"`
	MarkPrice        float64 `json:"markPrice"`
	UnrealizedProfit float64 `json:"unrealizedProfit"`
	Leverage         float64 `json:"leverage"`
	Notional         float64 `json:"notional"`
	LiquidationPrice float64 `json:"liquidationPrice"`
}
