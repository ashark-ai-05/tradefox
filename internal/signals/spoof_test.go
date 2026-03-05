package signals

import "testing"

func TestSpoof_LargeOrderVanished(t *testing.T) {
	prev := LevelMap{100: 50, 99: 5, 98: 5}
	curr := []BookLevel{{99, 5}, {98, 5}}
	s := ComputeSpoof(prev, nil, curr, nil)
	if !s.Active {
		t.Error("expected spoof detected when large bid vanished")
	}
	if s.Side != "bid" {
		t.Errorf("side = %s, want bid", s.Side)
	}
}

func TestSpoof_NormalCancel(t *testing.T) {
	prev := LevelMap{100: 5, 99: 5}
	curr := []BookLevel{{99, 5}}
	s := ComputeSpoof(prev, nil, curr, nil)
	if s.Active {
		t.Error("expected no spoof for normal-sized cancel")
	}
}
