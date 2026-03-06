package scanner

import (
	"math"
	"testing"
)

func TestLiqMath_LongLiq(t *testing.T) {
	// Long liq = entry * (1 - 1/leverage)
	entry := 100.0

	tests := []struct {
		leverage float64
		expected float64
	}{
		{3, 100 * (1 - 1.0/3)},     // 66.67
		{5, 100 * (1 - 1.0/5)},     // 80
		{10, 100 * (1 - 1.0/10)},   // 90
		{20, 100 * (1 - 1.0/20)},   // 95
		{25, 100 * (1 - 1.0/25)},   // 96
		{50, 100 * (1 - 1.0/50)},   // 98
		{100, 100 * (1 - 1.0/100)}, // 99
	}

	for _, tt := range tests {
		result := entry * (1 - 1/tt.leverage)
		if math.Abs(result-tt.expected) > 0.01 {
			t.Errorf("Long liq at %fx leverage: got %f, want %f", tt.leverage, result, tt.expected)
		}
	}
}

func TestLiqMath_ShortLiq(t *testing.T) {
	// Short liq = entry * (1 + 1/leverage)
	entry := 100.0

	tests := []struct {
		leverage float64
		expected float64
	}{
		{3, 100 * (1 + 1.0/3)},     // 133.33
		{5, 100 * (1 + 1.0/5)},     // 120
		{10, 100 * (1 + 1.0/10)},   // 110
		{20, 100 * (1 + 1.0/20)},   // 105
		{100, 100 * (1 + 1.0/100)}, // 101
	}

	for _, tt := range tests {
		result := entry * (1 + 1/tt.leverage)
		if math.Abs(result-tt.expected) > 0.01 {
			t.Errorf("Short liq at %fx leverage: got %f, want %f", tt.leverage, result, tt.expected)
		}
	}
}

func TestEstimateLiqClusters_BasicCandles(t *testing.T) {
	currentPrice := 100.0
	candles := []Candle{
		{Open: 98, High: 102, Low: 97, Close: 101, Volume: 1000},
		{Open: 101, High: 105, Low: 100, Close: 103, Volume: 1500},
		{Open: 103, High: 106, Low: 99, Close: 100, Volume: 2000},
	}

	result := EstimateLiqClusters(currentPrice, candles)

	if result.AbovePrice <= 0 {
		t.Error("expected some above price volume")
	}
	if result.BelowPrice <= 0 {
		t.Error("expected some below price volume")
	}
	if result.NearestAbove.Price <= currentPrice {
		t.Errorf("nearest above (%f) should be > current price (%f)", result.NearestAbove.Price, currentPrice)
	}
	if result.NearestBelow.Price >= currentPrice {
		t.Errorf("nearest below (%f) should be < current price (%f)", result.NearestBelow.Price, currentPrice)
	}
	if result.Asymmetry <= 0 {
		t.Error("asymmetry should be positive")
	}
}

func TestEstimateLiqClusters_EmptyCandles(t *testing.T) {
	result := EstimateLiqClusters(100, nil)
	if result.AbovePrice != 0 || result.BelowPrice != 0 {
		t.Error("expected zero values for empty candles")
	}
}

func TestEstimateLiqClusters_ZeroPrice(t *testing.T) {
	candles := []Candle{{Open: 100, High: 110, Low: 90, Close: 100, Volume: 1000}}
	result := EstimateLiqClusters(0, candles)
	if result.AbovePrice != 0 {
		t.Error("expected zero for zero price")
	}
}
