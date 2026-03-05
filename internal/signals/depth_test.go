package signals

import "testing"

func TestDepthImbalance_BidHeavy(t *testing.T) {
	bids := []BookLevel{{100, 10}, {99, 8}, {98, 6}}
	asks := []BookLevel{{101, 2}, {102, 3}, {103, 4}}
	s := ComputeDepthImbalance(bids, asks, 0)
	if s.Weighted <= 0 {
		t.Errorf("expected bid-heavy, got weighted=%f", s.Weighted)
	}
}

func TestDepthImbalance_Balanced(t *testing.T) {
	bids := []BookLevel{{100, 5}, {99, 5}}
	asks := []BookLevel{{101, 5}, {102, 5}}
	s := ComputeDepthImbalance(bids, asks, 0)
	if s.Pressure != "neutral" {
		t.Errorf("expected neutral, got %s (weighted=%f)", s.Pressure, s.Weighted)
	}
}
