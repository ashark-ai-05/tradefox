package signals

import (
	"testing"
	"time"
)

func TestVolatility_Regime(t *testing.T) {
	now := time.Now()
	trades := make([]TradeRecord, 30)
	for i := range trades {
		trades[i] = TradeRecord{
			Price: 100 + float64(i)*0.0001, Size: 1, IsBuy: true,
			Timestamp: now.Add(time.Duration(i) * time.Second),
		}
	}
	state := &VolState{}
	s := ComputeVolatility(trades, 0, state)
	if s.Regime == "extreme" || s.Regime == "high" {
		t.Errorf("expected low/normal vol with tiny price changes, got %s (%.1f%%)", s.Regime, s.Realized)
	}
}
