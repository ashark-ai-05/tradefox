package execution

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/ashark-ai-05/tradefox/internal/core/enums"
	"github.com/ashark-ai-05/tradefox/internal/core/interfaces"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

// mockTrader implements interfaces.Trader for testing.
type mockTrader struct {
	placeOrderFn    func(ctx context.Context, order *models.Order) (*models.Order, error)
	cancelOrderFn   func(ctx context.Context, symbol string, orderID int64) error
	getOpenOrdersFn func(ctx context.Context, symbol string) ([]models.Order, error)
}

func (m *mockTrader) PlaceOrder(ctx context.Context, order *models.Order) (*models.Order, error) {
	if m.placeOrderFn != nil {
		return m.placeOrderFn(ctx, order)
	}
	result := order.Clone()
	result.OrderID = 12345
	result.Status = enums.OrderStatusNew
	return result, nil
}

func (m *mockTrader) CancelOrder(ctx context.Context, symbol string, orderID int64) error {
	if m.cancelOrderFn != nil {
		return m.cancelOrderFn(ctx, symbol, orderID)
	}
	return nil
}

func (m *mockTrader) ModifyOrder(ctx context.Context, symbol string, orderID int64, newQty, newPrice float64) (*models.Order, error) {
	return &models.Order{OrderID: orderID, Symbol: symbol, Quantity: newQty, PricePlaced: newPrice, Status: enums.OrderStatusReplaced}, nil
}

func (m *mockTrader) GetOpenOrders(ctx context.Context, symbol string) ([]models.Order, error) {
	if m.getOpenOrdersFn != nil {
		return m.getOpenOrdersFn(ctx, symbol)
	}
	return []models.Order{}, nil
}

func (m *mockTrader) GetOrderHistory(ctx context.Context, symbol string, limit int) ([]models.Order, error) {
	return []models.Order{}, nil
}

func (m *mockTrader) GetPositions(ctx context.Context) ([]interfaces.ExchangePosition, error) {
	return []interfaces.ExchangePosition{}, nil
}

func (m *mockTrader) ClosePosition(ctx context.Context, symbol string, quantity float64) (*models.Order, error) {
	return &models.Order{Symbol: symbol, Quantity: quantity, Status: enums.OrderStatusFilled}, nil
}

func TestExecutor_PlaceOrder_Success(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	risk := NewRiskManager(RiskLimits{MaxPositionSize: 100}, logger)
	exec := NewExecutor(risk, bus, logger)
	exec.RegisterTrader("test", &mockTrader{})

	order := &models.Order{
		Symbol:      "BTCUSDT",
		Side:        enums.OrderSideBuy,
		OrderType:   enums.OrderTypeMarket,
		Quantity:    1,
		PricePlaced: 50000,
	}

	result, err := exec.PlaceOrder(context.Background(), "test", order)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OrderID != 12345 {
		t.Fatalf("expected orderID 12345, got %d", result.OrderID)
	}
	if result.Status != enums.OrderStatusNew {
		t.Fatalf("expected status New, got %v", result.Status)
	}
}

func TestExecutor_PlaceOrder_RiskRejection(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	risk := NewRiskManager(RiskLimits{MaxPositionSize: 1}, logger)
	exec := NewExecutor(risk, bus, logger)
	exec.RegisterTrader("test", &mockTrader{})

	order := &models.Order{
		Symbol:   "BTCUSDT",
		Quantity: 10, // exceeds max of 1
	}

	_, err := exec.PlaceOrder(context.Background(), "test", order)
	if err == nil {
		t.Fatal("expected risk rejection error")
	}
}

func TestExecutor_PlaceOrder_NoTrader(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	risk := NewRiskManager(RiskLimits{MaxPositionSize: 100}, logger)
	exec := NewExecutor(risk, bus, logger)

	order := &models.Order{Symbol: "BTCUSDT", Quantity: 1}

	_, err := exec.PlaceOrder(context.Background(), "nonexistent", order)
	if err == nil {
		t.Fatal("expected error for missing trader")
	}
}

func TestExecutor_CancelOrder(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	risk := NewRiskManager(RiskLimits{}, logger)
	exec := NewExecutor(risk, bus, logger)
	exec.RegisterTrader("test", &mockTrader{})

	err := exec.CancelOrder(context.Background(), "test", "BTCUSDT", 12345)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecutor_KillSwitch_BlocksOrders(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	risk := NewRiskManager(RiskLimits{}, logger)
	risk.ActivateKillSwitch()
	exec := NewExecutor(risk, bus, logger)
	exec.RegisterTrader("test", &mockTrader{})

	order := &models.Order{Symbol: "BTCUSDT", Quantity: 1}
	_, err := exec.PlaceOrder(context.Background(), "test", order)
	if err == nil {
		t.Fatal("expected kill switch to block order")
	}
}

func TestExecutor_RegisterAndGetTrader(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	bus := eventbus.NewBus(logger)
	defer bus.Close()

	risk := NewRiskManager(RiskLimits{}, logger)
	exec := NewExecutor(risk, bus, logger)

	_, ok := exec.GetTrader("test")
	if ok {
		t.Fatal("expected no trader before registration")
	}

	exec.RegisterTrader("test", &mockTrader{})
	_, ok = exec.GetTrader("test")
	if !ok {
		t.Fatal("expected trader after registration")
	}
}
