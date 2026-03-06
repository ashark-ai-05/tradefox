package execution

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/ashark-ai-05/tradefox/internal/core/interfaces"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

// Executor routes orders to the correct exchange connector's Trader interface
// and wraps them with pre-trade risk checks.
type Executor struct {
	mu      sync.RWMutex
	traders map[string]interfaces.Trader // exchange name → Trader
	risk    *RiskManager
	bus     *eventbus.Bus
	logger  *slog.Logger
}

// NewExecutor creates a new Executor.
func NewExecutor(risk *RiskManager, bus *eventbus.Bus, logger *slog.Logger) *Executor {
	return &Executor{
		traders: make(map[string]interfaces.Trader),
		risk:    risk,
		bus:     bus,
		logger:  logger,
	}
}

// RegisterTrader registers a Trader implementation for an exchange name.
func (e *Executor) RegisterTrader(exchange string, trader interfaces.Trader) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.traders[exchange] = trader
}

// GetTrader returns the trader for the given exchange, if registered.
func (e *Executor) GetTrader(exchange string) (interfaces.Trader, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	t, ok := e.traders[exchange]
	return t, ok
}

// PlaceOrder validates the order against risk limits and routes it to the appropriate trader.
func (e *Executor) PlaceOrder(ctx context.Context, exchange string, order *models.Order) (*models.Order, error) {
	if err := e.risk.CheckPreTrade(order); err != nil {
		return nil, fmt.Errorf("risk check failed: %w", err)
	}

	trader, ok := e.GetTrader(exchange)
	if !ok {
		return nil, fmt.Errorf("no trader registered for exchange %q", exchange)
	}

	result, err := trader.PlaceOrder(ctx, order)
	if err != nil {
		return nil, fmt.Errorf("place order failed: %w", err)
	}

	e.logger.Info("order placed",
		slog.String("exchange", exchange),
		slog.String("symbol", result.Symbol),
		slog.Int64("order_id", result.OrderID),
		slog.String("side", result.Side.String()),
	)

	// Publish order event to event bus
	e.bus.Positions.Publish(*result)

	return result, nil
}

// CancelOrder cancels an order on the given exchange.
func (e *Executor) CancelOrder(ctx context.Context, exchange, symbol string, orderID int64) error {
	trader, ok := e.GetTrader(exchange)
	if !ok {
		return fmt.Errorf("no trader registered for exchange %q", exchange)
	}

	if err := trader.CancelOrder(ctx, symbol, orderID); err != nil {
		return fmt.Errorf("cancel order failed: %w", err)
	}

	e.logger.Info("order cancelled",
		slog.String("exchange", exchange),
		slog.String("symbol", symbol),
		slog.Int64("order_id", orderID),
	)

	return nil
}

// ModifyOrder modifies an existing order.
func (e *Executor) ModifyOrder(ctx context.Context, exchange, symbol string, orderID int64, newQty, newPrice float64) (*models.Order, error) {
	trader, ok := e.GetTrader(exchange)
	if !ok {
		return nil, fmt.Errorf("no trader registered for exchange %q", exchange)
	}

	result, err := trader.ModifyOrder(ctx, symbol, orderID, newQty, newPrice)
	if err != nil {
		return nil, fmt.Errorf("modify order failed: %w", err)
	}

	e.bus.Positions.Publish(*result)
	return result, nil
}

// GetOpenOrders returns open orders from the given exchange.
func (e *Executor) GetOpenOrders(ctx context.Context, exchange, symbol string) ([]models.Order, error) {
	trader, ok := e.GetTrader(exchange)
	if !ok {
		return nil, fmt.Errorf("no trader registered for exchange %q", exchange)
	}
	return trader.GetOpenOrders(ctx, symbol)
}

// GetOrderHistory returns historical orders from the given exchange.
func (e *Executor) GetOrderHistory(ctx context.Context, exchange, symbol string, limit int) ([]models.Order, error) {
	trader, ok := e.GetTrader(exchange)
	if !ok {
		return nil, fmt.Errorf("no trader registered for exchange %q", exchange)
	}
	return trader.GetOrderHistory(ctx, symbol, limit)
}

// GetPositions returns positions from the given exchange.
func (e *Executor) GetPositions(ctx context.Context, exchange string) ([]interfaces.ExchangePosition, error) {
	trader, ok := e.GetTrader(exchange)
	if !ok {
		return nil, fmt.Errorf("no trader registered for exchange %q", exchange)
	}
	return trader.GetPositions(ctx)
}

// ClosePosition closes a position on the given exchange.
func (e *Executor) ClosePosition(ctx context.Context, exchange, symbol string, quantity float64) (*models.Order, error) {
	trader, ok := e.GetTrader(exchange)
	if !ok {
		return nil, fmt.Errorf("no trader registered for exchange %q", exchange)
	}

	result, err := trader.ClosePosition(ctx, symbol, quantity)
	if err != nil {
		return nil, fmt.Errorf("close position failed: %w", err)
	}

	e.bus.Positions.Publish(*result)
	return result, nil
}

// Risk returns the underlying RiskManager.
func (e *Executor) Risk() *RiskManager {
	return e.risk
}
