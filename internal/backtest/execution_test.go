package backtest

import (
	"math"
	"testing"
)

func TestExecutionSimulator_FillAtNextBook(t *testing.T) {
	exec := NewExecutionSimulator(DefaultExecutionConfig())
	exec.PlaceOrder("SOLUSDT", 1, 1.0, 1000)

	// Same timestamp -- should NOT fill
	fill := exec.TryFill(100.0, 0.10, 0.001, 1000)
	if fill != nil {
		t.Error("should not fill at same timestamp")
	}

	// Next timestamp -- should fill
	fill = exec.TryFill(100.0, 0.10, 0.001, 1100)
	if fill == nil {
		t.Fatal("expected fill")
	}
	// Fill price = 100 + 0.05 (half spread) + 1.0*0.001*1.0 (slippage) = 100.051
	if fill.Price < 100.0 {
		t.Error("fill should be above mid for buy")
	}
	if fill.Fees <= 0 {
		t.Error("expected positive fees")
	}
}

func TestExecutionSimulator_SellFill(t *testing.T) {
	exec := NewExecutionSimulator(DefaultExecutionConfig())
	exec.PlaceOrder("SOLUSDT", -1, 1.0, 1000)

	fill := exec.TryFill(100.0, 0.10, 0.001, 1100)
	if fill == nil {
		t.Fatal("expected fill")
	}
	// Sell: price = 100 - 0.05 (half spread) - slippage
	if fill.Price > 100.0 {
		t.Error("fill should be below mid for sell")
	}
}

func TestSlippageModel(t *testing.T) {
	exec := NewExecutionSimulator(ExecutionConfig{TakerFeeBps: 5.0, SpreadMult: 2.0})
	exec.PlaceOrder("SOLUSDT", 1, 10.0, 1000)
	fill := exec.TryFill(100.0, 0.10, 0.005, 1100)
	if fill == nil {
		t.Fatal("expected fill")
	}
	// Slippage = 10.0 * 0.005 * 2.0 = 0.10
	expectedSlippage := 10.0 * 0.005 * 2.0
	if math.Abs(fill.Slippage-expectedSlippage) > 0.001 {
		t.Errorf("expected slippage %.4f, got %.4f", expectedSlippage, fill.Slippage)
	}
}

func TestExecutionSimulator_NoPending(t *testing.T) {
	exec := NewExecutionSimulator(DefaultExecutionConfig())
	fill := exec.TryFill(100.0, 0.10, 0.001, 1100)
	if fill != nil {
		t.Error("should return nil with no pending order")
	}
}

func TestExecutionSimulator_HasPending(t *testing.T) {
	exec := NewExecutionSimulator(DefaultExecutionConfig())
	if exec.HasPending() {
		t.Error("should not have pending initially")
	}
	exec.PlaceOrder("SOLUSDT", 1, 1.0, 1000)
	if !exec.HasPending() {
		t.Error("should have pending after PlaceOrder")
	}
}

func TestExecutionSimulator_Cancel(t *testing.T) {
	exec := NewExecutionSimulator(DefaultExecutionConfig())
	exec.PlaceOrder("SOLUSDT", 1, 1.0, 1000)
	exec.Cancel()
	if exec.HasPending() {
		t.Error("should not have pending after cancel")
	}
	fill := exec.TryFill(100.0, 0.10, 0.001, 1100)
	if fill != nil {
		t.Error("should not fill after cancel")
	}
}

func TestExecutionSimulator_FillClearsPending(t *testing.T) {
	exec := NewExecutionSimulator(DefaultExecutionConfig())
	exec.PlaceOrder("SOLUSDT", 1, 1.0, 1000)
	exec.TryFill(100.0, 0.10, 0.001, 1100)
	if exec.HasPending() {
		t.Error("pending should be cleared after fill")
	}
}

func TestExecutionSimulator_FeeCalculation(t *testing.T) {
	exec := NewExecutionSimulator(ExecutionConfig{TakerFeeBps: 10.0, SpreadMult: 0})
	exec.PlaceOrder("SOLUSDT", 1, 2.0, 1000)
	fill := exec.TryFill(100.0, 0.0, 0.0, 1100)
	if fill == nil {
		t.Fatal("expected fill")
	}
	// Notional = 100.0 * 2.0 = 200, fees = 200 * 10/10000 = 0.20
	expectedFees := 200.0 * 10.0 / 10000.0
	if math.Abs(fill.Fees-expectedFees) > 0.001 {
		t.Errorf("expected fees %.4f, got %.4f", expectedFees, fill.Fees)
	}
}
