package signals

import (
	"testing"
	"time"
)

func TestSweepDetection_BuySweep(t *testing.T) {
	now := time.Now()
	trades := make([]TradeRecord, 10)
	for i := 0; i < 5; i++ {
		trades[i] = TradeRecord{Price: 100, Size: 1, IsBuy: true, Timestamp: now.Add(-10 * time.Second)}
	}
	for i := 5; i < 10; i++ {
		trades[i] = TradeRecord{
			Price:     100 + float64(i-5)*0.1,
			Size:      2,
			IsBuy:     true,
			Timestamp: now.Add(-time.Duration(200-i*20) * time.Millisecond),
		}
	}
	s := ComputeSweep(trades)
	if !s.Active {
		t.Error("expected sweep to be active")
	}
	if s.Dir != "buy" {
		t.Errorf("dir = %s, want buy", s.Dir)
	}
}

func TestComputeSweepAt(t *testing.T) {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	trades := make([]TradeRecord, 10)
	for i := range trades {
		trades[i] = TradeRecord{
			Price:     100.0 + float64(i)*0.1,
			Size:      1.0,
			IsBuy:     true,
			Timestamp: now.Add(-time.Duration(10-i) * 100 * time.Millisecond),
		}
	}
	result := ComputeSweepAt(trades, now)
	if !result.Active {
		t.Error("expected sweep to be active")
	}
	if result.Dir != "buy" {
		t.Errorf("expected buy, got %s", result.Dir)
	}
}

func TestSweepDetection_NoSweep(t *testing.T) {
	now := time.Now()
	trades := make([]TradeRecord, 5)
	for i := range trades {
		trades[i] = TradeRecord{Price: 100, Size: 1, IsBuy: true, Timestamp: now}
	}
	s := ComputeSweep(trades)
	if s.Active {
		t.Error("expected no sweep with same price")
	}
}
