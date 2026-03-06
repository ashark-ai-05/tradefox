package execution

import (
	"log/slog"
	"os"
	"testing"

	"github.com/ashark-ai-05/tradefox/internal/core/enums"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestCheckPreTrade_PassesValidOrder(t *testing.T) {
	rm := NewRiskManager(RiskLimits{
		MaxPositionSize: 10,
		MaxNotional:     500_000,
		DailyLossLimit:  5_000,
	}, testLogger())

	order := &models.Order{
		Symbol:      "BTCUSDT",
		Quantity:    5,
		PricePlaced: 50_000,
		Side:        enums.OrderSideBuy,
		OrderType:   enums.OrderTypeLimit,
	}

	if err := rm.CheckPreTrade(order); err != nil {
		t.Fatalf("expected order to pass risk check, got: %v", err)
	}
}

func TestCheckPreTrade_RejectsOversizedPosition(t *testing.T) {
	rm := NewRiskManager(RiskLimits{
		MaxPositionSize: 5,
		MaxNotional:     0,
		DailyLossLimit:  0,
	}, testLogger())

	order := &models.Order{
		Symbol:   "BTCUSDT",
		Quantity: 10,
	}

	if err := rm.CheckPreTrade(order); err == nil {
		t.Fatal("expected oversized position to be rejected")
	}
}

func TestCheckPreTrade_RejectsExcessiveNotional(t *testing.T) {
	rm := NewRiskManager(RiskLimits{
		MaxPositionSize: 100,
		MaxNotional:     50_000,
		DailyLossLimit:  0,
	}, testLogger())

	order := &models.Order{
		Symbol:      "BTCUSDT",
		Quantity:    2,
		PricePlaced: 50_000, // notional = 100_000 > 50_000 limit
	}

	if err := rm.CheckPreTrade(order); err == nil {
		t.Fatal("expected excessive notional to be rejected")
	}
}

func TestCheckPreTrade_MarketOrderWithZeroPrice(t *testing.T) {
	rm := NewRiskManager(RiskLimits{
		MaxPositionSize: 100,
		MaxNotional:     50_000,
	}, testLogger())

	order := &models.Order{
		Symbol:      "BTCUSDT",
		Quantity:    1,
		PricePlaced: 0, // market order, price unknown
		OrderType:   enums.OrderTypeMarket,
	}

	if err := rm.CheckPreTrade(order); err != nil {
		t.Fatalf("market order with 0 price should pass: %v", err)
	}
}

func TestKillSwitch_BlocksTrading(t *testing.T) {
	rm := NewRiskManager(RiskLimits{
		MaxPositionSize: 100,
	}, testLogger())

	rm.ActivateKillSwitch()

	order := &models.Order{
		Symbol:   "BTCUSDT",
		Quantity: 1,
	}

	if err := rm.CheckPreTrade(order); err == nil {
		t.Fatal("expected order to be blocked by kill switch")
	}
}

func TestKillSwitch_ManualToggle(t *testing.T) {
	rm := NewRiskManager(RiskLimits{}, testLogger())

	if rm.IsKillSwitchActive() {
		t.Fatal("kill switch should start inactive")
	}

	rm.ActivateKillSwitch()
	if !rm.IsKillSwitchActive() {
		t.Fatal("kill switch should be active")
	}

	rm.DeactivateKillSwitch()
	if rm.IsKillSwitchActive() {
		t.Fatal("kill switch should be deactivated")
	}
}

func TestUpdateOnFill_TriggersKillSwitch(t *testing.T) {
	rm := NewRiskManager(RiskLimits{
		DailyLossLimit: 100,
	}, testLogger())

	rm.UpdateOnFill(-50)
	if rm.IsKillSwitchActive() {
		t.Fatal("kill switch should not trigger at -50 (limit is -100)")
	}

	rm.UpdateOnFill(-60) // total = -110
	if !rm.IsKillSwitchActive() {
		t.Fatal("kill switch should trigger when daily loss exceeds limit")
	}
}

func TestStatus_ReturnsCurrentState(t *testing.T) {
	rm := NewRiskManager(RiskLimits{
		DailyLossLimit: 500,
	}, testLogger())

	rm.UpdateOnFill(-100)
	status := rm.Status()

	if status.DailyPnL != -100 {
		t.Fatalf("expected dailyPnL -100, got %f", status.DailyPnL)
	}
	if status.DailyLossLimit != 500 {
		t.Fatalf("expected dailyLossLimit 500, got %f", status.DailyLossLimit)
	}
	if status.KillSwitch {
		t.Fatal("kill switch should not be active")
	}
}
