package signals

import (
	"testing"
	"time"
)

func TestLambda_HighImpact(t *testing.T) {
	now := time.Now()
	trades := make([]TradeRecord, 50)
	for i := range trades {
		trades[i] = TradeRecord{
			Price:     100 + float64(i)*0.01,
			Size:      1,
			IsBuy:     true,
			Timestamp: now.Add(time.Duration(i) * time.Second),
		}
	}
	s := ComputeLambda(trades, 0)
	if s.Value <= 0 {
		t.Errorf("expected positive lambda with trending price, got %f", s.Value)
	}
}

func TestLambda_InsufficientTrades(t *testing.T) {
	trades := make([]TradeRecord, 5)
	s := ComputeLambda(trades, 1.5)
	if s.Value != 1.5 {
		t.Errorf("expected previous value 1.5 with insufficient trades, got %f", s.Value)
	}
}
