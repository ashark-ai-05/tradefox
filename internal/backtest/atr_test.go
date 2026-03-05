package backtest

import (
	"math"
	"testing"
)

func TestATR(t *testing.T) {
	atr := NewATRCalculator(3)
	atr.Update(105, 95, 100) // TR = 10
	atr.Update(108, 98, 103) // TR = max(10, 8, 5) = 10
	atr.Update(106, 96, 101) // TR = max(10, 3, 7) = 10
	if math.Abs(atr.Current()-10.0) > 0.01 {
		t.Errorf("expected ATR=10, got %f", atr.Current())
	}
}

func TestATR_FirstCandle(t *testing.T) {
	atr := NewATRCalculator(14)
	result := atr.Update(110, 90, 100)
	// First candle: TR = high - low = 20
	if math.Abs(result-20.0) > 0.01 {
		t.Errorf("expected ATR=20 for first candle, got %f", result)
	}
}

func TestATR_SlidingWindow(t *testing.T) {
	atr := NewATRCalculator(2)
	atr.Update(110, 90, 100) // TR = 20
	atr.Update(105, 95, 100) // TR = max(10, 5, 5) = 10
	// ATR = (20+10)/2 = 15
	if math.Abs(atr.Current()-15.0) > 0.01 {
		t.Errorf("expected ATR=15, got %f", atr.Current())
	}

	atr.Update(102, 98, 100) // TR = max(4, 2, 2) = 4
	// Window size 2: only last 2 TRs: (10+4)/2 = 7
	if math.Abs(atr.Current()-7.0) > 0.01 {
		t.Errorf("expected ATR=7, got %f", atr.Current())
	}
}

func TestATR_Current(t *testing.T) {
	atr := NewATRCalculator(5)
	if atr.Current() != 0 {
		t.Errorf("expected 0 for fresh ATR, got %f", atr.Current())
	}
	atr.Update(110, 90, 100)
	if atr.Current() == 0 {
		t.Error("expected non-zero ATR after update")
	}
}
