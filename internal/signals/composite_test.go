package signals

import "testing"

func TestComposite_Bullish(t *testing.T) {
	micro := MicropriceSignal{Dir: "up"}
	ofi := OFISignal{Value: 0.5}
	depth := DepthImbSignal{Weighted: 0.3}
	sweep := SweepSignal{Active: true, Dir: "buy"}
	s := ComputeComposite(micro, ofi, depth, sweep, 0)
	if s.Dir != "BULLISH" {
		t.Errorf("dir = %s, want BULLISH (avg=%f)", s.Dir, s.Avg)
	}
}

func TestComposite_Neutral(t *testing.T) {
	micro := MicropriceSignal{Dir: "neutral"}
	ofi := OFISignal{Value: 0}
	depth := DepthImbSignal{Weighted: 0}
	sweep := SweepSignal{}
	s := ComputeComposite(micro, ofi, depth, sweep, 0)
	if s.Dir != "NEUTRAL" {
		t.Errorf("dir = %s, want NEUTRAL", s.Dir)
	}
}
