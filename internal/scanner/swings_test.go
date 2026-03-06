package scanner

import "testing"

func TestDetectSwingPoints_Basic(t *testing.T) {
	candles := []Candle{
		{High: 100, Low: 95},
		{High: 105, Low: 98},  // Swing high
		{High: 103, Low: 96},
		{High: 101, Low: 92},  // Swing low
		{High: 104, Low: 94},
		{High: 107, Low: 97},  // Swing high
		{High: 106, Low: 95},
	}

	swings := DetectSwingPoints(candles)
	if len(swings) == 0 {
		t.Fatal("expected swing points to be detected")
	}

	// Check that we find both swing highs and lows
	var highs, lows int
	for _, s := range swings {
		if s.Type == "SwingHigh" {
			highs++
		} else if s.Type == "SwingLow" {
			lows++
		}
	}

	if highs == 0 {
		t.Error("expected at least one swing high")
	}
	if lows == 0 {
		t.Error("expected at least one swing low")
	}
}

func TestDetectSwingPoints_Classification(t *testing.T) {
	// Create candles where a swing forms early (many confirmations = C3)
	candles := make([]Candle, 10)
	for i := range candles {
		candles[i] = Candle{High: 100, Low: 95}
	}
	// Create a clear swing high at index 2
	candles[1].High = 90
	candles[2].High = 110
	candles[3].High = 95

	swings := DetectSwingPoints(candles)
	found := false
	for _, s := range swings {
		if s.Type == "SwingHigh" && s.Price == 110 {
			found = true
			if s.Class != "C3" {
				t.Errorf("expected C3 for well-confirmed swing, got %s", s.Class)
			}
		}
	}
	if !found {
		t.Error("expected to find swing high at price 110")
	}
}

func TestDetectSwingPoints_TooFewCandles(t *testing.T) {
	candles := []Candle{{High: 100, Low: 95}, {High: 105, Low: 98}}
	swings := DetectSwingPoints(candles)
	if len(swings) != 0 {
		t.Errorf("expected 0 swings for 2 candles, got %d", len(swings))
	}
}

func TestGetLatestSwing(t *testing.T) {
	candles := []Candle{
		{High: 100, Low: 95},
		{High: 105, Low: 98},
		{High: 103, Low: 92},
		{High: 101, Low: 90},
		{High: 104, Low: 94},
	}

	result := GetLatestSwing(candles)
	if result.Type == "" {
		t.Error("expected a swing result")
	}
}
