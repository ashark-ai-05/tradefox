package scanner

import (
	"math"
	"testing"
)

func TestDetectFVGs_Bullish(t *testing.T) {
	// Bullish FVG: candle[0].High < candle[2].Low
	candles := []Candle{
		{Open: 100, High: 102, Low: 99, Close: 101},
		{Open: 101, High: 105, Low: 100, Close: 104}, // middle candle
		{Open: 104, High: 108, Low: 103, Close: 107},  // candle[2].Low (103) > candle[0].High (102)
	}
	fvgs := DetectFVGs(candles, "1d")
	if len(fvgs) != 1 {
		t.Fatalf("expected 1 FVG, got %d", len(fvgs))
	}
	if fvgs[0].Type != "Bullish" {
		t.Errorf("expected Bullish, got %s", fvgs[0].Type)
	}
	if fvgs[0].Low != 102 || fvgs[0].High != 103 {
		t.Errorf("expected gap 102-103, got %f-%f", fvgs[0].Low, fvgs[0].High)
	}
}

func TestDetectFVGs_Bearish(t *testing.T) {
	// Bearish FVG: candle[0].Low > candle[2].High
	candles := []Candle{
		{Open: 108, High: 110, Low: 106, Close: 107},
		{Open: 107, High: 108, Low: 103, Close: 104}, // middle candle
		{Open: 104, High: 105, Low: 100, Close: 101},  // candle[2].High (105) < candle[0].Low (106)
	}
	fvgs := DetectFVGs(candles, "4h")
	if len(fvgs) != 1 {
		t.Fatalf("expected 1 FVG, got %d", len(fvgs))
	}
	if fvgs[0].Type != "Bearish" {
		t.Errorf("expected Bearish, got %s", fvgs[0].Type)
	}
}

func TestDetectFVGs_None(t *testing.T) {
	// No gap — overlapping candles
	candles := []Candle{
		{Open: 100, High: 105, Low: 99, Close: 103},
		{Open: 103, High: 106, Low: 102, Close: 105},
		{Open: 105, High: 104, Low: 100, Close: 102},
	}
	fvgs := DetectFVGs(candles, "1d")
	if len(fvgs) != 0 {
		t.Errorf("expected 0 FVGs, got %d", len(fvgs))
	}
}

func TestFindNearestFVG(t *testing.T) {
	fvgs := []FVG{
		{High: 105, Low: 103, Type: "Bullish", Timeframe: "1d", Filled: false, FillPct: 0},
		{High: 115, Low: 112, Type: "Bearish", Timeframe: "4h", Filled: false, FillPct: 0},
		{High: 120, Low: 118, Type: "Bullish", Timeframe: "1d", Filled: true, FillPct: 100}, // filled
	}

	result := FindNearestFVG(fvgs, 106)
	if result.Type != "Bullish" {
		t.Errorf("expected nearest to be Bullish, got %s", result.Type)
	}
	// Midpoint of nearest FVG is 104, current price 106
	expectedProx := ((104.0 - 106.0) / 106.0) * 100
	if math.Abs(result.Proximity-expectedProx) > 0.01 {
		t.Errorf("expected proximity %f, got %f", expectedProx, result.Proximity)
	}
}

func TestFindNearestFVG_Empty(t *testing.T) {
	result := FindNearestFVG(nil, 100)
	if result.Type != "" {
		t.Errorf("expected empty result for nil FVGs")
	}
}

func TestFVGFillDetection(t *testing.T) {
	// Create candles with a bullish FVG that gets partially filled
	candles := []Candle{
		{Open: 100, High: 102, Low: 99, Close: 101},
		{Open: 101, High: 106, Low: 100, Close: 105},
		{Open: 105, High: 110, Low: 103, Close: 109}, // gap: 102-103
		{Open: 109, High: 110, Low: 102.5, Close: 108}, // partially fills gap
	}
	fvgs := DetectFVGs(candles, "1d")
	if len(fvgs) == 0 {
		t.Fatal("expected at least 1 FVG")
	}

	// Should have some fill percentage
	if fvgs[0].FillPct <= 0 {
		t.Error("expected FVG to be partially filled")
	}
}
