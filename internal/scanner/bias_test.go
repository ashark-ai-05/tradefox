package scanner

import "testing"

func TestCalcBias_Uptrend(t *testing.T) {
	// Create zigzag candles with higher highs and higher lows (uptrend)
	// Pattern: up, up, down, up, up, down (each cycle higher than previous)
	candles := []Candle{
		{High: 100, Low: 95, Close: 98},
		{High: 102, Low: 97, Close: 101},
		{High: 105, Low: 99, Close: 103},  // swing high 1 @ 105
		{High: 103, Low: 98, Close: 100},
		{High: 101, Low: 97, Close: 99},   // swing low 1 @ 97
		{High: 104, Low: 99, Close: 103},
		{High: 108, Low: 101, Close: 106}, // swing high 2 @ 108 > 105
		{High: 105, Low: 100, Close: 102},
		{High: 103, Low: 99, Close: 101},  // swing low 2 @ 99 > 97
		{High: 106, Low: 101, Close: 105},
		{High: 112, Low: 104, Close: 110}, // swing high 3 @ 112 > 108
		{High: 108, Low: 103, Close: 105},
		{High: 106, Low: 102, Close: 104}, // swing low 3 @ 102 > 99
		{High: 110, Low: 105, Close: 109},
		{High: 115, Low: 107, Close: 113},
	}

	result := CalcBias(candles)
	if result.Direction != "High" {
		t.Errorf("expected High bias for uptrend, got %s", result.Direction)
	}
}

func TestCalcBias_Downtrend(t *testing.T) {
	// Zigzag candles with lower highs and lower lows (downtrend)
	candles := []Candle{
		{High: 115, Low: 108, Close: 112},
		{High: 113, Low: 107, Close: 110},
		{High: 110, Low: 106, Close: 108}, // swing high 1 @ 115 (from candle 0)
		{High: 108, Low: 103, Close: 105},
		{High: 106, Low: 101, Close: 103}, // swing low 1 @ 101
		{High: 109, Low: 103, Close: 107},
		{High: 111, Low: 105, Close: 108}, // swing high 2 @ 111 < 115
		{High: 107, Low: 100, Close: 102},
		{High: 104, Low: 98, Close: 100},  // swing low 2 @ 98 < 101
		{High: 106, Low: 100, Close: 104},
		{High: 108, Low: 102, Close: 105}, // swing high 3 @ 108 < 111
		{High: 103, Low: 96, Close: 98},
		{High: 100, Low: 94, Close: 96},   // swing low 3 @ 94 < 98
		{High: 102, Low: 96, Close: 99},
		{High: 98, Low: 92, Close: 94},
	}

	result := CalcBias(candles)
	if result.Direction != "Low" {
		t.Errorf("expected Low bias for downtrend, got %s", result.Direction)
	}
}

func TestCalcBias_InsufficientData(t *testing.T) {
	candles := make([]Candle, 5)
	result := CalcBias(candles)
	if result.Direction != "None" {
		t.Errorf("expected None for insufficient data, got %s", result.Direction)
	}
}

func TestCalcBias_Sideways(t *testing.T) {
	// Oscillating candles
	candles := make([]Candle, 30)
	for i := range candles {
		if i%4 < 2 {
			candles[i] = Candle{Open: 100, High: 105, Low: 95, Close: 102}
		} else {
			candles[i] = Candle{Open: 102, High: 105, Low: 95, Close: 98}
		}
	}

	result := CalcBias(candles)
	_ = result // Should not panic
}
