package signals

import (
	"math"
	"testing"
)

func TestComputeOFI_BidReinforce(t *testing.T) {
	prev := OFIState{BestBidPrice: 100, BestBidSize: 5, BestAskPrice: 101, BestAskSize: 5}
	sig, _ := ComputeOFI(prev, 100, 8, 101, 5, 0) // bid size grew
	if sig.Value <= 0 {
		t.Errorf("OFI should be positive when bid reinforced, got %f", sig.Value)
	}
}

func TestComputeOFI_BidPriceUp(t *testing.T) {
	prev := OFIState{BestBidPrice: 100, BestBidSize: 5, BestAskPrice: 101, BestAskSize: 5}
	sig, _ := ComputeOFI(prev, 100.5, 3, 101, 5, 0) // bid price moved up
	if sig.Value <= 0 {
		t.Errorf("OFI should be positive when bid price up, got %f", sig.Value)
	}
}

func TestComputeOFI_Clamped(t *testing.T) {
	prev := OFIState{BestBidPrice: 100, BestBidSize: 100, BestAskPrice: 101, BestAskSize: 1}
	sig, _ := ComputeOFI(prev, 100.5, 100, 101.5, 1, 0)
	if math.Abs(sig.Value) > 1 {
		t.Errorf("OFI should be clamped to [-1,1], got %f", sig.Value)
	}
}

func TestComputeOFI_NoPrevious(t *testing.T) {
	prev := OFIState{} // zero state
	sig, _ := ComputeOFI(prev, 100, 5, 101, 5, 0)
	if sig.Value != 0 {
		t.Errorf("OFI with no previous state should be 0, got %f", sig.Value)
	}
}
