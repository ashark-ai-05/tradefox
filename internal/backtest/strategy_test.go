package backtest

import (
	"testing"

	"github.com/ashark-ai-05/tradefox/internal/signals"
)

func TestComputeConfluence_StrongLong(t *testing.T) {
	cfg := DefaultStrategyConfig()
	evt := SignalEvent{
		Signals: &signals.SignalSet{
			OFI:        signals.OFISignal{Value: 0.8},
			Microprice: signals.MicropriceSignal{DivBps: 2.0},
			DepthImb:   signals.DepthImbSignal{Weighted: 0.6},
			Sweep:      signals.SweepSignal{Active: true, Dir: "buy"},
			Lambda:     signals.LambdaSignal{Regime: "low"},
			Vol:        signals.VolSignal{Regime: "low"},
			Composite:  signals.CompositeSignal{Avg: 0.7},
		},
	}
	result := ComputeConfluence(cfg, evt, 5)
	if result.Direction != 1 {
		t.Errorf("expected long, got %d", result.Direction)
	}
	if result.Score < cfg.ConfluenceThreshold {
		t.Errorf("expected score above threshold, got %f", result.Score)
	}
	if result.Vetoed {
		t.Error("should not be vetoed")
	}
}

func TestComputeConfluence_StrongShort(t *testing.T) {
	cfg := DefaultStrategyConfig()
	evt := SignalEvent{
		Signals: &signals.SignalSet{
			OFI:        signals.OFISignal{Value: -0.8},
			Microprice: signals.MicropriceSignal{DivBps: -2.0},
			DepthImb:   signals.DepthImbSignal{Weighted: -0.6},
			Sweep:      signals.SweepSignal{Active: true, Dir: "sell"},
			Lambda:     signals.LambdaSignal{Regime: "low"},
			Vol:        signals.VolSignal{Regime: "low"},
			Composite:  signals.CompositeSignal{Avg: -0.7},
		},
	}
	result := ComputeConfluence(cfg, evt, 5)
	if result.Direction != -1 {
		t.Errorf("expected short, got %d", result.Direction)
	}
	if result.Score < cfg.ConfluenceThreshold {
		t.Errorf("expected score above threshold, got %f", result.Score)
	}
}

func TestComputeConfluence_VetoExtreme(t *testing.T) {
	cfg := DefaultStrategyConfig()
	evt := SignalEvent{
		Signals: &signals.SignalSet{
			OFI: signals.OFISignal{Value: 0.5},
			Vol: signals.VolSignal{Regime: "extreme"},
		},
	}
	result := ComputeConfluence(cfg, evt, 5)
	if !result.Vetoed {
		t.Error("should be vetoed for extreme vol")
	}
	if result.VetoReason != "extreme volatility regime" {
		t.Errorf("wrong veto reason: %s", result.VetoReason)
	}
}

func TestComputeConfluence_VetoDisabled(t *testing.T) {
	cfg := DefaultStrategyConfig()
	cfg.VetoExtreme = false
	evt := SignalEvent{
		Signals: &signals.SignalSet{
			OFI: signals.OFISignal{Value: 0.5},
			Vol: signals.VolSignal{Regime: "extreme"},
		},
	}
	result := ComputeConfluence(cfg, evt, 5)
	if result.Vetoed {
		t.Error("should not be vetoed when VetoExtreme is false")
	}
}

func TestComputeConfluence_NoSignal(t *testing.T) {
	cfg := DefaultStrategyConfig()
	evt := SignalEvent{
		Signals: &signals.SignalSet{
			OFI:        signals.OFISignal{Value: 0.0},
			Microprice: signals.MicropriceSignal{DivBps: 0.0},
			DepthImb:   signals.DepthImbSignal{Weighted: 0.0},
			Lambda:     signals.LambdaSignal{Regime: "medium"},
			Vol:        signals.VolSignal{Regime: "medium"},
			Composite:  signals.CompositeSignal{Avg: 0.0},
		},
	}
	result := ComputeConfluence(cfg, evt, 0)
	// Vol "medium" contributes +0.25 for B6, so there's a slight long bias.
	// Score should still be very low (well below confluence threshold).
	if result.Score >= cfg.ConfluenceThreshold {
		t.Errorf("expected score below threshold, got %f", result.Score)
	}
}

func TestOFITracker_Persistence(t *testing.T) {
	tracker := NewOFITracker()

	// First positive reading
	c := tracker.Update("SOLUSDT", 0.1)
	if c != 1 {
		t.Errorf("expected 1, got %d", c)
	}

	// Second positive
	c = tracker.Update("SOLUSDT", 0.2)
	if c != 2 {
		t.Errorf("expected 2, got %d", c)
	}

	// Third positive
	c = tracker.Update("SOLUSDT", 0.15)
	if c != 3 {
		t.Errorf("expected 3, got %d", c)
	}

	// Direction flip
	c = tracker.Update("SOLUSDT", -0.1)
	if c != 1 {
		t.Errorf("expected reset to 1, got %d", c)
	}
}

func TestOFITracker_NeutralResets(t *testing.T) {
	tracker := NewOFITracker()

	tracker.Update("SOLUSDT", 0.1) // positive
	tracker.Update("SOLUSDT", 0.2) // positive
	c := tracker.Update("SOLUSDT", 0.0) // neutral (within +/-0.05 threshold)
	if c != 1 {
		t.Errorf("neutral should reset counter, expected 1, got %d", c)
	}
}

func TestComputeConfluence_OFIPersistenceBelowThreshold(t *testing.T) {
	cfg := DefaultStrategyConfig()
	evt := SignalEvent{
		Signals: &signals.SignalSet{
			OFI: signals.OFISignal{Value: 0.5},
		},
	}
	// ofiPersistence < MinOFIPersistence (3), so B9 should be 0
	result := ComputeConfluence(cfg, evt, 2)
	if result.Components[8] != 0 {
		t.Errorf("B9 should be 0 with persistence below threshold, got %f", result.Components[8])
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		v, min, max, want float64
	}{
		{0.5, -1, 1, 0.5},
		{-2.0, -1, 1, -1.0},
		{3.0, -1, 1, 1.0},
		{0.0, -1, 1, 0.0},
	}
	for _, tt := range tests {
		got := clamp(tt.v, tt.min, tt.max)
		if got != tt.want {
			t.Errorf("clamp(%f, %f, %f) = %f, want %f", tt.v, tt.min, tt.max, got, tt.want)
		}
	}
}
