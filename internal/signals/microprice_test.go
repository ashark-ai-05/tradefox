package signals

import "testing"

func TestComputeMicroprice_Neutral(t *testing.T) {
	s := ComputeMicroprice(100.0, 100.0, 0)
	if s.Dir != "neutral" {
		t.Errorf("dir = %s, want neutral", s.Dir)
	}
	if s.DivBps != 0 {
		t.Errorf("divBps = %f, want 0", s.DivBps)
	}
}

func TestComputeMicroprice_Up(t *testing.T) {
	// microprice 1 bps above mid
	s := ComputeMicroprice(100.0, 100.01, 0)
	if s.Dir != "up" {
		t.Errorf("dir = %s, want up (divBps=%f)", s.Dir, s.DivBps)
	}
}

func TestComputeMicroprice_Down(t *testing.T) {
	s := ComputeMicroprice(100.0, 99.99, 0)
	if s.Dir != "down" {
		t.Errorf("dir = %s, want down (divBps=%f)", s.Dir, s.DivBps)
	}
}

func TestComputeMicroprice_EMASmoothing(t *testing.T) {
	// First call with large divergence
	s1 := ComputeMicroprice(100.0, 100.01, 0) // ~1 bps
	// Second call with zero divergence, should decay via EMA
	s2 := ComputeMicroprice(100.0, 100.0, s1.DivBps)
	if s2.DivBps >= s1.DivBps {
		t.Errorf("EMA should decay: s1=%f, s2=%f", s1.DivBps, s2.DivBps)
	}
}
