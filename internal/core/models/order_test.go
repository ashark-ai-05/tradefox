package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/core/enums"
)

// ---------------------------------------------------------------------------
// PendingQuantity tests
// ---------------------------------------------------------------------------

func TestOrder_PendingQuantity_Normal(t *testing.T) {
	o := &Order{
		Quantity:       100,
		FilledQuantity: 30,
		Status:         enums.OrderStatusNew,
	}
	got := o.PendingQuantity()
	want := 70.0
	if got != want {
		t.Errorf("PendingQuantity() = %v, want %v", got, want)
	}
}

func TestOrder_PendingQuantity_Canceled(t *testing.T) {
	o := &Order{
		Quantity:       100,
		FilledQuantity: 30,
		Status:         enums.OrderStatusCanceled,
	}
	got := o.PendingQuantity()
	if got != 0 {
		t.Errorf("PendingQuantity() = %v, want 0 (status=Canceled)", got)
	}
}

func TestOrder_PendingQuantity_Filled(t *testing.T) {
	o := &Order{
		Quantity:       100,
		FilledQuantity: 100,
		Status:         enums.OrderStatusFilled,
	}
	got := o.PendingQuantity()
	if got != 0 {
		t.Errorf("PendingQuantity() = %v, want 0 (status=Filled)", got)
	}
}

// ---------------------------------------------------------------------------
// Update test
// ---------------------------------------------------------------------------

func TestOrder_Update(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond).UTC()

	src := &Order{
		OrderID:              12345,
		Symbol:               "EUR/USD",
		ProviderID:           7,
		ProviderName:         "TestProvider",
		ClOrdID:              "cl-001",
		Side:                 enums.OrderSideBuy,
		OrderType:            enums.OrderTypeLimit,
		TimeInForce:          enums.TimeInForceGTC,
		Status:               enums.OrderStatusPartialFilled,
		Quantity:             1000,
		MinQuantity:          100,
		FilledQuantity:       250,
		PricePlaced:          1.2345,
		Currency:             "USD",
		FutSettDate:          "20260301",
		IsMM:                 true,
		IsEmpty:              false,
		LayerName:            "Layer1",
		AttemptsToClose:      3,
		SymbolMultiplier:     100,
		SymbolDecimals:       5,
		FreeText:             "test order",
		OriginPartyID:        "party-1",
		Executions:           []Execution{{OrderID: 12345, ExecutionID: "ex-1", Price: 1.2340, QtyFilled: 250}},
		QuoteID:              42,
		QuoteServerTimestamp: now.Add(-10 * time.Millisecond),
		QuoteLocalTimestamp:  now.Add(-5 * time.Millisecond),
		CreationTimestamp:    now.Add(-time.Hour),
		LastUpdated:          now,
		ExecutedTimestamp:    now.Add(-30 * time.Minute),
		FireSignalTimestamp:  now.Add(-time.Hour - time.Minute),
		StopLoss:             1.2200,
		TakeProfit:           1.2500,
		PipsTrail:            true,
		UnrealizedPnL:        150.75,
		MaxDrawdown:          50.25,
		BestBid:              1.2344,
		BestAsk:              1.2346,
		FilledPrice:          1.2340,
		FilledPercentage:     25.0,
	}

	dst := &Order{}
	dst.Update(src)

	// Verify all fields match.
	if dst.OrderID != src.OrderID {
		t.Errorf("OrderID: got %d, want %d", dst.OrderID, src.OrderID)
	}
	if dst.Symbol != src.Symbol {
		t.Errorf("Symbol: got %q, want %q", dst.Symbol, src.Symbol)
	}
	if dst.ProviderID != src.ProviderID {
		t.Errorf("ProviderID: got %d, want %d", dst.ProviderID, src.ProviderID)
	}
	if dst.ProviderName != src.ProviderName {
		t.Errorf("ProviderName: got %q, want %q", dst.ProviderName, src.ProviderName)
	}
	if dst.ClOrdID != src.ClOrdID {
		t.Errorf("ClOrdID: got %q, want %q", dst.ClOrdID, src.ClOrdID)
	}
	if dst.Side != src.Side {
		t.Errorf("Side: got %v, want %v", dst.Side, src.Side)
	}
	if dst.OrderType != src.OrderType {
		t.Errorf("OrderType: got %v, want %v", dst.OrderType, src.OrderType)
	}
	if dst.TimeInForce != src.TimeInForce {
		t.Errorf("TimeInForce: got %v, want %v", dst.TimeInForce, src.TimeInForce)
	}
	if dst.Status != src.Status {
		t.Errorf("Status: got %v, want %v", dst.Status, src.Status)
	}
	if dst.Quantity != src.Quantity {
		t.Errorf("Quantity: got %v, want %v", dst.Quantity, src.Quantity)
	}
	if dst.MinQuantity != src.MinQuantity {
		t.Errorf("MinQuantity: got %v, want %v", dst.MinQuantity, src.MinQuantity)
	}
	if dst.FilledQuantity != src.FilledQuantity {
		t.Errorf("FilledQuantity: got %v, want %v", dst.FilledQuantity, src.FilledQuantity)
	}
	if dst.PricePlaced != src.PricePlaced {
		t.Errorf("PricePlaced: got %v, want %v", dst.PricePlaced, src.PricePlaced)
	}
	if dst.Currency != src.Currency {
		t.Errorf("Currency: got %q, want %q", dst.Currency, src.Currency)
	}
	if dst.FutSettDate != src.FutSettDate {
		t.Errorf("FutSettDate: got %q, want %q", dst.FutSettDate, src.FutSettDate)
	}
	if dst.IsMM != src.IsMM {
		t.Errorf("IsMM: got %v, want %v", dst.IsMM, src.IsMM)
	}
	if dst.IsEmpty != src.IsEmpty {
		t.Errorf("IsEmpty: got %v, want %v", dst.IsEmpty, src.IsEmpty)
	}
	if dst.LayerName != src.LayerName {
		t.Errorf("LayerName: got %q, want %q", dst.LayerName, src.LayerName)
	}
	if dst.AttemptsToClose != src.AttemptsToClose {
		t.Errorf("AttemptsToClose: got %d, want %d", dst.AttemptsToClose, src.AttemptsToClose)
	}
	if dst.SymbolMultiplier != src.SymbolMultiplier {
		t.Errorf("SymbolMultiplier: got %d, want %d", dst.SymbolMultiplier, src.SymbolMultiplier)
	}
	if dst.SymbolDecimals != src.SymbolDecimals {
		t.Errorf("SymbolDecimals: got %d, want %d", dst.SymbolDecimals, src.SymbolDecimals)
	}
	if dst.FreeText != src.FreeText {
		t.Errorf("FreeText: got %q, want %q", dst.FreeText, src.FreeText)
	}
	if dst.OriginPartyID != src.OriginPartyID {
		t.Errorf("OriginPartyID: got %q, want %q", dst.OriginPartyID, src.OriginPartyID)
	}
	if len(dst.Executions) != len(src.Executions) {
		t.Errorf("Executions length: got %d, want %d", len(dst.Executions), len(src.Executions))
	}
	if dst.QuoteID != src.QuoteID {
		t.Errorf("QuoteID: got %d, want %d", dst.QuoteID, src.QuoteID)
	}
	if !dst.QuoteServerTimestamp.Equal(src.QuoteServerTimestamp) {
		t.Errorf("QuoteServerTimestamp: got %v, want %v", dst.QuoteServerTimestamp, src.QuoteServerTimestamp)
	}
	if !dst.QuoteLocalTimestamp.Equal(src.QuoteLocalTimestamp) {
		t.Errorf("QuoteLocalTimestamp: got %v, want %v", dst.QuoteLocalTimestamp, src.QuoteLocalTimestamp)
	}
	if !dst.CreationTimestamp.Equal(src.CreationTimestamp) {
		t.Errorf("CreationTimestamp: got %v, want %v", dst.CreationTimestamp, src.CreationTimestamp)
	}
	if !dst.LastUpdated.Equal(src.LastUpdated) {
		t.Errorf("LastUpdated: got %v, want %v", dst.LastUpdated, src.LastUpdated)
	}
	if !dst.ExecutedTimestamp.Equal(src.ExecutedTimestamp) {
		t.Errorf("ExecutedTimestamp: got %v, want %v", dst.ExecutedTimestamp, src.ExecutedTimestamp)
	}
	if !dst.FireSignalTimestamp.Equal(src.FireSignalTimestamp) {
		t.Errorf("FireSignalTimestamp: got %v, want %v", dst.FireSignalTimestamp, src.FireSignalTimestamp)
	}
	if dst.StopLoss != src.StopLoss {
		t.Errorf("StopLoss: got %v, want %v", dst.StopLoss, src.StopLoss)
	}
	if dst.TakeProfit != src.TakeProfit {
		t.Errorf("TakeProfit: got %v, want %v", dst.TakeProfit, src.TakeProfit)
	}
	if dst.PipsTrail != src.PipsTrail {
		t.Errorf("PipsTrail: got %v, want %v", dst.PipsTrail, src.PipsTrail)
	}
	if dst.UnrealizedPnL != src.UnrealizedPnL {
		t.Errorf("UnrealizedPnL: got %v, want %v", dst.UnrealizedPnL, src.UnrealizedPnL)
	}
	if dst.MaxDrawdown != src.MaxDrawdown {
		t.Errorf("MaxDrawdown: got %v, want %v", dst.MaxDrawdown, src.MaxDrawdown)
	}
	if dst.BestBid != src.BestBid {
		t.Errorf("BestBid: got %v, want %v", dst.BestBid, src.BestBid)
	}
	if dst.BestAsk != src.BestAsk {
		t.Errorf("BestAsk: got %v, want %v", dst.BestAsk, src.BestAsk)
	}
	if dst.FilledPrice != src.FilledPrice {
		t.Errorf("FilledPrice: got %v, want %v", dst.FilledPrice, src.FilledPrice)
	}
	if dst.FilledPercentage != src.FilledPercentage {
		t.Errorf("FilledPercentage: got %v, want %v", dst.FilledPercentage, src.FilledPercentage)
	}
}

// ---------------------------------------------------------------------------
// Reset test
// ---------------------------------------------------------------------------

func TestOrder_Reset(t *testing.T) {
	now := time.Now().UTC()
	o := &Order{
		OrderID:           99,
		Symbol:            "BTC/USD",
		ProviderID:        3,
		ProviderName:      "Exchange",
		Side:              enums.OrderSideSell,
		OrderType:         enums.OrderTypeMarket,
		Status:            enums.OrderStatusFilled,
		Quantity:           500,
		FilledQuantity:    500,
		PricePlaced:       42000.50,
		IsMM:              true,
		Executions:        []Execution{{OrderID: 99, ExecutionID: "ex-1"}},
		CreationTimestamp: now,
		LastUpdated:       now,
	}

	o.Reset()

	zero := Order{}
	if o.OrderID != zero.OrderID {
		t.Errorf("OrderID: got %d, want %d", o.OrderID, zero.OrderID)
	}
	if o.Symbol != zero.Symbol {
		t.Errorf("Symbol: got %q, want %q", o.Symbol, zero.Symbol)
	}
	if o.ProviderID != zero.ProviderID {
		t.Errorf("ProviderID: got %d, want %d", o.ProviderID, zero.ProviderID)
	}
	if o.ProviderName != zero.ProviderName {
		t.Errorf("ProviderName: got %q, want %q", o.ProviderName, zero.ProviderName)
	}
	if o.Side != zero.Side {
		t.Errorf("Side: got %v, want %v", o.Side, zero.Side)
	}
	if o.OrderType != zero.OrderType {
		t.Errorf("OrderType: got %v, want %v", o.OrderType, zero.OrderType)
	}
	if o.Status != zero.Status {
		t.Errorf("Status: got %v, want %v", o.Status, zero.Status)
	}
	if o.Quantity != zero.Quantity {
		t.Errorf("Quantity: got %v, want %v", o.Quantity, zero.Quantity)
	}
	if o.FilledQuantity != zero.FilledQuantity {
		t.Errorf("FilledQuantity: got %v, want %v", o.FilledQuantity, zero.FilledQuantity)
	}
	if o.PricePlaced != zero.PricePlaced {
		t.Errorf("PricePlaced: got %v, want %v", o.PricePlaced, zero.PricePlaced)
	}
	if o.IsMM != zero.IsMM {
		t.Errorf("IsMM: got %v, want %v", o.IsMM, zero.IsMM)
	}
	if o.Executions != nil {
		t.Error("Executions should be nil after Reset")
	}
	if !o.CreationTimestamp.IsZero() {
		t.Errorf("CreationTimestamp should be zero after Reset, got %v", o.CreationTimestamp)
	}
	if !o.LastUpdated.IsZero() {
		t.Errorf("LastUpdated should be zero after Reset, got %v", o.LastUpdated)
	}
}

// ---------------------------------------------------------------------------
// Clone tests
// ---------------------------------------------------------------------------

func TestOrder_Clone(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond).UTC()
	original := &Order{
		OrderID:           42,
		Symbol:            "AAPL",
		ProviderID:        1,
		ProviderName:      "Provider1",
		Side:              enums.OrderSideBuy,
		OrderType:         enums.OrderTypeLimit,
		Status:            enums.OrderStatusNew,
		Quantity:           200,
		FilledQuantity:    50,
		PricePlaced:       150.25,
		CreationTimestamp: now,
		LastUpdated:       now,
		StopLoss:          145.00,
		TakeProfit:        160.00,
	}

	clone := original.Clone()

	// Verify fields match.
	if clone.OrderID != original.OrderID {
		t.Errorf("OrderID: got %d, want %d", clone.OrderID, original.OrderID)
	}
	if clone.Symbol != original.Symbol {
		t.Errorf("Symbol: got %q, want %q", clone.Symbol, original.Symbol)
	}
	if clone.Quantity != original.Quantity {
		t.Errorf("Quantity: got %v, want %v", clone.Quantity, original.Quantity)
	}
	if clone.StopLoss != original.StopLoss {
		t.Errorf("StopLoss: got %v, want %v", clone.StopLoss, original.StopLoss)
	}

	// Modify original; clone must be unaffected.
	original.OrderID = 999
	original.Symbol = "CHANGED"
	original.Quantity = 9999
	original.StopLoss = 0

	if clone.OrderID == 999 {
		t.Error("Clone OrderID should not change when original is modified")
	}
	if clone.Symbol == "CHANGED" {
		t.Error("Clone Symbol should not change when original is modified")
	}
	if clone.Quantity == 9999 {
		t.Error("Clone Quantity should not change when original is modified")
	}
	if clone.StopLoss == 0 {
		t.Error("Clone StopLoss should not change when original is modified")
	}
}

func TestOrder_Clone_Executions(t *testing.T) {
	original := &Order{
		OrderID: 10,
		Symbol:  "MSFT",
		Executions: []Execution{
			{OrderID: 10, ExecutionID: "ex-1", Price: 300.50, QtyFilled: 100},
			{OrderID: 10, ExecutionID: "ex-2", Price: 301.00, QtyFilled: 50},
		},
	}

	clone := original.Clone()

	// Verify executions are copied.
	if len(clone.Executions) != 2 {
		t.Fatalf("clone Executions length: got %d, want 2", len(clone.Executions))
	}
	if clone.Executions[0].ExecutionID != "ex-1" {
		t.Errorf("clone Executions[0].ExecutionID: got %q, want %q", clone.Executions[0].ExecutionID, "ex-1")
	}
	if clone.Executions[1].ExecutionID != "ex-2" {
		t.Errorf("clone Executions[1].ExecutionID: got %q, want %q", clone.Executions[1].ExecutionID, "ex-2")
	}

	// Modify original executions; clone must be unaffected.
	original.Executions[0].Price = 999.99
	original.Executions = append(original.Executions, Execution{ExecutionID: "ex-3"})

	if clone.Executions[0].Price == 999.99 {
		t.Error("Clone execution price should not change when original is modified")
	}
	if len(clone.Executions) != 2 {
		t.Errorf("Clone executions length should still be 2, got %d", len(clone.Executions))
	}
}

// ---------------------------------------------------------------------------
// JSON round-trip test
// ---------------------------------------------------------------------------

func TestOrder_JSONRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond).UTC()

	original := &Order{
		OrderID:              12345,
		Symbol:               "EUR/USD",
		ProviderID:           7,
		ProviderName:         "TestProvider",
		ClOrdID:              "cl-001",
		Side:                 enums.OrderSideBuy,
		OrderType:            enums.OrderTypeLimit,
		TimeInForce:          enums.TimeInForceGTC,
		Status:               enums.OrderStatusPartialFilled,
		Quantity:             1000,
		MinQuantity:          100,
		FilledQuantity:       250,
		PricePlaced:          1.2345,
		Currency:             "USD",
		FutSettDate:          "20260301",
		IsMM:                 true,
		IsEmpty:              false,
		LayerName:            "Layer1",
		AttemptsToClose:      3,
		SymbolMultiplier:     100,
		SymbolDecimals:       5,
		FreeText:             "test order",
		OriginPartyID:        "party-1",
		Executions:           []Execution{{OrderID: 12345, ExecutionID: "ex-1", Price: 1.2340, QtyFilled: 250, Side: enums.OrderSideBuy, Status: enums.OrderStatusPartialFilled, Timestamp: now, ServerTimestamp: now.Add(-time.Millisecond), LocalTimestamp: now}},
		QuoteID:              42,
		QuoteServerTimestamp: now.Add(-10 * time.Millisecond),
		QuoteLocalTimestamp:  now.Add(-5 * time.Millisecond),
		CreationTimestamp:    now.Add(-time.Hour),
		LastUpdated:          now,
		ExecutedTimestamp:    now.Add(-30 * time.Minute),
		FireSignalTimestamp:  now.Add(-time.Hour - time.Minute),
		StopLoss:             1.2200,
		TakeProfit:           1.2500,
		PipsTrail:            true,
		UnrealizedPnL:        150.75,
		MaxDrawdown:          50.25,
		BestBid:              1.2344,
		BestAsk:              1.2346,
		FilledPrice:          1.2340,
		FilledPercentage:     25.0,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var restored Order
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// Verify scalar fields.
	if restored.OrderID != original.OrderID {
		t.Errorf("OrderID: got %d, want %d", restored.OrderID, original.OrderID)
	}
	if restored.Symbol != original.Symbol {
		t.Errorf("Symbol: got %q, want %q", restored.Symbol, original.Symbol)
	}
	if restored.ProviderID != original.ProviderID {
		t.Errorf("ProviderID: got %d, want %d", restored.ProviderID, original.ProviderID)
	}
	if restored.ProviderName != original.ProviderName {
		t.Errorf("ProviderName: got %q, want %q", restored.ProviderName, original.ProviderName)
	}
	if restored.ClOrdID != original.ClOrdID {
		t.Errorf("ClOrdID: got %q, want %q", restored.ClOrdID, original.ClOrdID)
	}

	// Verify enum fields.
	if restored.Side != original.Side {
		t.Errorf("Side: got %v, want %v", restored.Side, original.Side)
	}
	if restored.OrderType != original.OrderType {
		t.Errorf("OrderType: got %v, want %v", restored.OrderType, original.OrderType)
	}
	if restored.TimeInForce != original.TimeInForce {
		t.Errorf("TimeInForce: got %v, want %v", restored.TimeInForce, original.TimeInForce)
	}
	if restored.Status != original.Status {
		t.Errorf("Status: got %v, want %v", restored.Status, original.Status)
	}

	// Verify numeric fields.
	if restored.Quantity != original.Quantity {
		t.Errorf("Quantity: got %v, want %v", restored.Quantity, original.Quantity)
	}
	if restored.FilledQuantity != original.FilledQuantity {
		t.Errorf("FilledQuantity: got %v, want %v", restored.FilledQuantity, original.FilledQuantity)
	}
	if restored.PricePlaced != original.PricePlaced {
		t.Errorf("PricePlaced: got %v, want %v", restored.PricePlaced, original.PricePlaced)
	}
	if restored.StopLoss != original.StopLoss {
		t.Errorf("StopLoss: got %v, want %v", restored.StopLoss, original.StopLoss)
	}
	if restored.TakeProfit != original.TakeProfit {
		t.Errorf("TakeProfit: got %v, want %v", restored.TakeProfit, original.TakeProfit)
	}
	if restored.FilledPrice != original.FilledPrice {
		t.Errorf("FilledPrice: got %v, want %v", restored.FilledPrice, original.FilledPrice)
	}
	if restored.FilledPercentage != original.FilledPercentage {
		t.Errorf("FilledPercentage: got %v, want %v", restored.FilledPercentage, original.FilledPercentage)
	}

	// Verify boolean fields.
	if restored.IsMM != original.IsMM {
		t.Errorf("IsMM: got %v, want %v", restored.IsMM, original.IsMM)
	}
	if restored.PipsTrail != original.PipsTrail {
		t.Errorf("PipsTrail: got %v, want %v", restored.PipsTrail, original.PipsTrail)
	}

	// Verify time fields.
	if !restored.CreationTimestamp.Equal(original.CreationTimestamp) {
		t.Errorf("CreationTimestamp: got %v, want %v", restored.CreationTimestamp, original.CreationTimestamp)
	}
	if !restored.LastUpdated.Equal(original.LastUpdated) {
		t.Errorf("LastUpdated: got %v, want %v", restored.LastUpdated, original.LastUpdated)
	}
	if !restored.ExecutedTimestamp.Equal(original.ExecutedTimestamp) {
		t.Errorf("ExecutedTimestamp: got %v, want %v", restored.ExecutedTimestamp, original.ExecutedTimestamp)
	}

	// Verify executions.
	if len(restored.Executions) != len(original.Executions) {
		t.Fatalf("Executions length: got %d, want %d", len(restored.Executions), len(original.Executions))
	}
	if restored.Executions[0].ExecutionID != original.Executions[0].ExecutionID {
		t.Errorf("Executions[0].ExecutionID: got %q, want %q", restored.Executions[0].ExecutionID, original.Executions[0].ExecutionID)
	}
	if restored.Executions[0].Price != original.Executions[0].Price {
		t.Errorf("Executions[0].Price: got %v, want %v", restored.Executions[0].Price, original.Executions[0].Price)
	}
	if restored.Executions[0].Side != original.Executions[0].Side {
		t.Errorf("Executions[0].Side: got %v, want %v", restored.Executions[0].Side, original.Executions[0].Side)
	}
}
