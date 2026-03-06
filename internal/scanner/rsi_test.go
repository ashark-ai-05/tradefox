package scanner

import (
	"math"
	"testing"
)

func TestCalcRSI_InsufficientData(t *testing.T) {
	candles := make([]Candle, 5)
	result := CalcRSI(candles, 14)
	if result != 50 {
		t.Errorf("expected 50 for insufficient data, got %f", result)
	}
}

func TestCalcRSI_AllGains(t *testing.T) {
	// 16 candles, each closing higher
	candles := make([]Candle, 16)
	for i := range candles {
		candles[i].Close = float64(100 + i)
	}
	result := CalcRSI(candles, 14)
	if result != 100 {
		t.Errorf("expected 100 for all-gains, got %f", result)
	}
}

func TestCalcRSI_AllLosses(t *testing.T) {
	candles := make([]Candle, 16)
	for i := range candles {
		candles[i].Close = float64(200 - i)
	}
	result := CalcRSI(candles, 14)
	if result != 0 {
		t.Errorf("expected 0 for all-losses, got %f", result)
	}
}

func TestCalcRSI_KnownSequence(t *testing.T) {
	// Alternating up/down should give RSI near 50
	candles := make([]Candle, 30)
	for i := range candles {
		if i%2 == 0 {
			candles[i].Close = 100
		} else {
			candles[i].Close = 101
		}
	}
	result := CalcRSI(candles, 14)
	if math.Abs(result-50) > 10 {
		t.Errorf("expected RSI near 50 for alternating, got %f", result)
	}
}

func TestCalcRSIHistory(t *testing.T) {
	candles := make([]Candle, 30)
	for i := range candles {
		candles[i].Close = float64(100 + i)
	}
	history := CalcRSIHistory(candles, 14, 10)
	if len(history) == 0 {
		t.Fatal("expected non-empty history")
	}
	if len(history) > 10 {
		t.Errorf("expected at most 10 history values, got %d", len(history))
	}
	// All should be 100 (all gains)
	for _, v := range history {
		if v != 100 {
			t.Errorf("expected 100, got %f", v)
		}
	}
}

func TestClassifyRSI(t *testing.T) {
	tests := []struct {
		value    float64
		expected string
	}{
		{10, "StrongOversold"},
		{25, "Oversold"},
		{35, "Weak"},
		{50, "Neutral"},
		{65, "Strong"},
		{75, "Overbought"},
		{85, "StrongOverbought"},
		{0, "StrongOversold"},
		{100, "StrongOverbought"},
		{19.99, "StrongOversold"},
		{20, "Oversold"},
		{59.99, "Neutral"},
		{60, "Strong"},
	}
	for _, tt := range tests {
		result := ClassifyRSI(tt.value)
		if result != tt.expected {
			t.Errorf("ClassifyRSI(%f) = %s, want %s", tt.value, result, tt.expected)
		}
	}
}
