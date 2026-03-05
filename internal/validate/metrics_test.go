package validate

import (
	"math"
	"testing"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/signals"
)

func TestPearsonCorrelation(t *testing.T) {
	// Perfect positive correlation
	xs := []float64{1, 2, 3, 4, 5}
	ys := []float64{2, 4, 6, 8, 10}
	r := pearson(xs, ys)
	if math.Abs(r-1.0) > 0.001 {
		t.Errorf("expected r~1.0, got %f", r)
	}

	// Perfect negative correlation
	ys2 := []float64{10, 8, 6, 4, 2}
	r2 := pearson(xs, ys2)
	if math.Abs(r2-(-1.0)) > 0.001 {
		t.Errorf("expected r~-1.0, got %f", r2)
	}

	// Zero correlation (constant y)
	ys3 := []float64{5, 5, 5, 5, 5}
	r3 := pearson(xs, ys3)
	if !math.IsNaN(r3) {
		t.Errorf("expected NaN for zero variance, got %f", r3)
	}

	// Insufficient data
	r4 := pearson([]float64{1}, []float64{2})
	if !math.IsNaN(r4) {
		t.Errorf("expected NaN for n<2, got %f", r4)
	}
}

func TestHitRate(t *testing.T) {
	horizon := 1 * time.Second
	rows := make([]ReturnRow, 100)
	for i := range rows {
		// Signal positive, return positive 70% of the time
		sig := 0.5
		ret := 0.001
		if i%10 < 3 {
			ret = -0.001 // 30% wrong
		}
		rows[i] = ReturnRow{
			Signals: &signals.SignalSet{OFI: signals.OFISignal{Value: sig}},
			Returns: map[time.Duration]float64{horizon: ret},
		}
	}

	rate, n := HitRate(rows, func(s *signals.SignalSet) float64 { return s.OFI.Value }, 0.1, horizon)
	if n != 100 {
		t.Errorf("expected n=100, got %d", n)
	}
	if math.Abs(rate-0.70) > 0.01 {
		t.Errorf("expected rate~0.70, got %f", rate)
	}
}

func TestHitRate_BelowThreshold(t *testing.T) {
	horizon := 1 * time.Second
	rows := []ReturnRow{
		{
			Signals: &signals.SignalSet{OFI: signals.OFISignal{Value: 0.01}}, // below threshold
			Returns: map[time.Duration]float64{horizon: 0.001},
		},
	}
	rate, n := HitRate(rows, func(s *signals.SignalSet) float64 { return s.OFI.Value }, 0.1, horizon)
	if n != 0 {
		t.Errorf("expected n=0, got %d", n)
	}
	if rate != 0 {
		t.Errorf("expected rate=0, got %f", rate)
	}
}

func TestDecayCurve(t *testing.T) {
	// Just verify it returns a map with the right keys
	horizons := []time.Duration{1 * time.Second, 5 * time.Second}
	rows := make([]ReturnRow, 50)
	for i := range rows {
		rows[i] = ReturnRow{
			Signals: &signals.SignalSet{OFI: signals.OFISignal{Value: float64(i)}},
			Returns: map[time.Duration]float64{
				1 * time.Second: float64(i) * 0.001,
				5 * time.Second: float64(i) * 0.002,
			},
		}
	}

	curve := DecayCurve(rows, func(s *signals.SignalSet) float64 { return s.OFI.Value }, horizons)
	if len(curve) != 2 {
		t.Errorf("expected 2 entries, got %d", len(curve))
	}
	// With linearly correlated data, correlation should be close to 1
	for h, c := range curve {
		if math.IsNaN(c) {
			t.Errorf("expected non-NaN correlation at horizon %s", h)
		}
		if c < 0.99 {
			t.Errorf("expected correlation > 0.99 at horizon %s, got %f", h, c)
		}
	}
}

func TestCorrelation_InsufficientData(t *testing.T) {
	horizon := 1 * time.Second
	rows := make([]ReturnRow, 10) // less than 30
	for i := range rows {
		rows[i] = ReturnRow{
			Signals: &signals.SignalSet{OFI: signals.OFISignal{Value: float64(i)}},
			Returns: map[time.Duration]float64{horizon: float64(i) * 0.001},
		}
	}

	c := Correlation(rows, func(s *signals.SignalSet) float64 { return s.OFI.Value }, horizon)
	if !math.IsNaN(c) {
		t.Errorf("expected NaN for insufficient data, got %f", c)
	}
}

func TestAllExtractors(t *testing.T) {
	extractors := AllExtractors()

	expectedNames := []string{"microprice", "ofi", "depth", "sweep", "lambda", "vol", "spoof", "composite"}
	for _, name := range expectedNames {
		if _, ok := extractors[name]; !ok {
			t.Errorf("missing extractor for %s", name)
		}
	}

	// Test each extractor with a sample signal set
	ss := &signals.SignalSet{
		Microprice: signals.MicropriceSignal{DivBps: 1.5},
		OFI:        signals.OFISignal{Value: 0.3},
		DepthImb:   signals.DepthImbSignal{Weighted: 0.2},
		Sweep:      signals.SweepSignal{Active: true, Dir: "buy"},
		Lambda:     signals.LambdaSignal{Value: 0.8},
		Vol:        signals.VolSignal{Realized: 0.05},
		Spoof:      signals.SpoofSignal{Score: 0.7},
		Composite:  signals.CompositeSignal{Avg: 0.4},
	}

	if v := extractors["microprice"](ss); v != 1.5 {
		t.Errorf("microprice: expected 1.5, got %f", v)
	}
	if v := extractors["ofi"](ss); v != 0.3 {
		t.Errorf("ofi: expected 0.3, got %f", v)
	}
	if v := extractors["depth"](ss); v != 0.2 {
		t.Errorf("depth: expected 0.2, got %f", v)
	}
	if v := extractors["sweep"](ss); v != 1.0 {
		t.Errorf("sweep (active buy): expected 1.0, got %f", v)
	}
	if v := extractors["lambda"](ss); v != 0.8 {
		t.Errorf("lambda: expected 0.8, got %f", v)
	}
	if v := extractors["vol"](ss); v != 0.05 {
		t.Errorf("vol: expected 0.05, got %f", v)
	}
	if v := extractors["spoof"](ss); v != 0.7 {
		t.Errorf("spoof: expected 0.7, got %f", v)
	}
	if v := extractors["composite"](ss); v != 0.4 {
		t.Errorf("composite: expected 0.4, got %f", v)
	}

	// Test sweep sell direction
	ss.Sweep = signals.SweepSignal{Active: true, Dir: "sell"}
	if v := extractors["sweep"](ss); v != -1.0 {
		t.Errorf("sweep (active sell): expected -1.0, got %f", v)
	}

	// Test sweep inactive
	ss.Sweep = signals.SweepSignal{Active: false}
	if v := extractors["sweep"](ss); v != 0.0 {
		t.Errorf("sweep (inactive): expected 0.0, got %f", v)
	}
}
