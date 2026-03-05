package models

import (
	"math"
	"sync"
	"testing"

	"github.com/ashark-ai-05/tradefox/internal/core/enums"
)

const floatTolerance = 1e-9

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < floatTolerance
}

// Helper to create an order with minimal fields needed for P&L tests.
func makeOrder(id int64, side enums.OrderSide, price, filledQty, totalQty float64, status enums.OrderStatus) Order {
	return Order{
		OrderID:        id,
		Side:           side,
		PricePlaced:    price,
		FilledQuantity: filledQty,
		Quantity:       totalQty,
		Status:         status,
		Symbol:         "TEST",
	}
}

// ---------------------------------------------------------------------------
// CalculateRealizedPnL tests
// ---------------------------------------------------------------------------

func TestCalculateRealizedPnL_FIFO(t *testing.T) {
	// Buy@100 qty=10, Buy@110 qty=5, Sell@120 qty=12
	// FIFO: match oldest buy first:
	//   10 @ (120-100) = 200
	//    2 @ (120-110) = 20
	// Total = 220
	buys := []Order{
		makeOrder(1, enums.OrderSideBuy, 100, 10, 10, enums.OrderStatusFilled),
		makeOrder(2, enums.OrderSideBuy, 110, 5, 5, enums.OrderStatusFilled),
	}
	sells := []Order{
		makeOrder(3, enums.OrderSideSell, 120, 12, 12, enums.OrderStatusFilled),
	}

	result := CalculateRealizedPnL(buys, sells, enums.PositionCalcFIFO)
	if !almostEqual(result, 220) {
		t.Errorf("FIFO realized PnL: got %f, want 220", result)
	}
}

func TestCalculateRealizedPnL_LIFO(t *testing.T) {
	// Buy@100 qty=10, Buy@110 qty=5, Sell@120 qty=12
	// LIFO: match newest buy first:
	//   5 @ (120-110) = 50
	//   7 @ (120-100) = 140
	// Total = 190
	buys := []Order{
		makeOrder(1, enums.OrderSideBuy, 100, 10, 10, enums.OrderStatusFilled),
		makeOrder(2, enums.OrderSideBuy, 110, 5, 5, enums.OrderStatusFilled),
	}
	sells := []Order{
		makeOrder(3, enums.OrderSideSell, 120, 12, 12, enums.OrderStatusFilled),
	}

	result := CalculateRealizedPnL(buys, sells, enums.PositionCalcLIFO)
	if !almostEqual(result, 190) {
		t.Errorf("LIFO realized PnL: got %f, want 190", result)
	}
}

func TestCalculateRealizedPnL_NoSells(t *testing.T) {
	buys := []Order{
		makeOrder(1, enums.OrderSideBuy, 100, 10, 10, enums.OrderStatusFilled),
	}

	result := CalculateRealizedPnL(buys, nil, enums.PositionCalcFIFO)
	if !almostEqual(result, 0) {
		t.Errorf("No sells realized PnL: got %f, want 0", result)
	}
}

func TestCalculateOpenPnL_FIFO(t *testing.T) {
	// Buy@100 qty=10, Buy@110 qty=5, Sell@120 qty=12
	// After FIFO matching: Buy@100 fully consumed, Buy@110 has 3 remaining
	// midPrice = 115
	// openPnL = 3 * (115 - 110) = 15
	buys := []Order{
		makeOrder(1, enums.OrderSideBuy, 100, 10, 10, enums.OrderStatusFilled),
		makeOrder(2, enums.OrderSideBuy, 110, 5, 5, enums.OrderStatusFilled),
	}
	sells := []Order{
		makeOrder(3, enums.OrderSideSell, 120, 12, 12, enums.OrderStatusFilled),
	}

	result := CalculateOpenPnL(buys, sells, enums.PositionCalcFIFO, 115)
	if !almostEqual(result, 15) {
		t.Errorf("FIFO open PnL: got %f, want 15", result)
	}
}

// ---------------------------------------------------------------------------
// Position tests
// ---------------------------------------------------------------------------

func TestPosition_AddOrder(t *testing.T) {
	pos := NewPosition("TEST", enums.PositionCalcFIFO)

	buy := makeOrder(1, enums.OrderSideBuy, 100, 10, 10, enums.OrderStatusFilled)
	pos.AddOrUpdateOrder(buy)

	if !almostEqual(pos.TotBuy, 10) {
		t.Errorf("TotBuy: got %f, want 10", pos.TotBuy)
	}
	if !almostEqual(pos.TotSell, 0) {
		t.Errorf("TotSell: got %f, want 0", pos.TotSell)
	}
}

func TestPosition_AddAndSellOrders(t *testing.T) {
	pos := NewPosition("TEST", enums.PositionCalcFIFO)

	buy := makeOrder(1, enums.OrderSideBuy, 100, 10, 10, enums.OrderStatusFilled)
	pos.AddOrUpdateOrder(buy)

	sell := makeOrder(2, enums.OrderSideSell, 120, 10, 10, enums.OrderStatusFilled)
	pos.AddOrUpdateOrder(sell)

	// Realized PnL = 10 * (120 - 100) = 200
	if !almostEqual(pos.PLRealized, 200) {
		t.Errorf("PLRealized: got %f, want 200", pos.PLRealized)
	}
	if !almostEqual(pos.TotBuy, 10) {
		t.Errorf("TotBuy: got %f, want 10", pos.TotBuy)
	}
	if !almostEqual(pos.TotSell, 10) {
		t.Errorf("TotSell: got %f, want 10", pos.TotSell)
	}
}

func TestPosition_UpdateMidPrice(t *testing.T) {
	pos := NewPosition("TEST", enums.PositionCalcFIFO)

	buy := makeOrder(1, enums.OrderSideBuy, 100, 10, 10, enums.OrderStatusFilled)
	pos.AddOrUpdateOrder(buy)

	// No sells, midPrice=110 -> openPnL = 10 * (110 - 100) = 100
	changed := pos.UpdateCurrentMidPrice(110)
	if !changed {
		t.Error("UpdateCurrentMidPrice should return true when price changes")
	}
	if !almostEqual(pos.PLOpen, 100) {
		t.Errorf("PLOpen: got %f, want 100", pos.PLOpen)
	}
	if !almostEqual(pos.PLTot, 100) {
		t.Errorf("PLTot: got %f, want 100", pos.PLTot)
	}

	// Setting same price should return false.
	changed = pos.UpdateCurrentMidPrice(110)
	if changed {
		t.Error("UpdateCurrentMidPrice should return false when price unchanged")
	}
}

func TestPosition_NetPosition(t *testing.T) {
	pos := NewPosition("TEST", enums.PositionCalcFIFO)

	buy := makeOrder(1, enums.OrderSideBuy, 100, 10, 10, enums.OrderStatusFilled)
	pos.AddOrUpdateOrder(buy)

	sell := makeOrder(2, enums.OrderSideSell, 120, 7, 7, enums.OrderStatusFilled)
	pos.AddOrUpdateOrder(sell)

	net := pos.NetPosition()
	if !almostEqual(net, 3) {
		t.Errorf("NetPosition: got %f, want 3", net)
	}
}

func TestPosition_Exposure(t *testing.T) {
	pos := NewPosition("TEST", enums.PositionCalcFIFO)

	buy := makeOrder(1, enums.OrderSideBuy, 100, 10, 10, enums.OrderStatusFilled)
	pos.AddOrUpdateOrder(buy)

	sell := makeOrder(2, enums.OrderSideSell, 120, 7, 7, enums.OrderStatusFilled)
	pos.AddOrUpdateOrder(sell)

	pos.UpdateCurrentMidPrice(115)

	// NetPosition = 10 - 7 = 3, Exposure = 3 * 115 = 345
	exposure := pos.Exposure()
	if !almostEqual(exposure, 345) {
		t.Errorf("Exposure: got %f, want 345", exposure)
	}
}

func TestPosition_ThreadSafety(t *testing.T) {
	pos := NewPosition("TEST", enums.PositionCalcFIFO)

	var wg sync.WaitGroup
	numGoroutines := 100

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			side := enums.OrderSideBuy
			if id%2 == 0 {
				side = enums.OrderSideSell
			}

			order := makeOrder(int64(id), side, float64(100+id), float64(id+1), float64(id+1), enums.OrderStatusFilled)
			pos.AddOrUpdateOrder(order)

			// Also exercise read methods concurrently.
			_ = pos.NetPosition()
			_ = pos.Exposure()
			_ = pos.GetAllOrders()
			pos.UpdateCurrentMidPrice(float64(110 + id))
		}(i)
	}

	wg.Wait()

	// Verify the position is in a consistent state: total orders should equal
	// numGoroutines since each goroutine adds a unique OrderID.
	allOrders := pos.GetAllOrders()
	if len(allOrders) != numGoroutines {
		t.Errorf("Expected %d total orders, got %d", numGoroutines, len(allOrders))
	}
}
