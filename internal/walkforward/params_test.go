package walkforward

import (
	"math"
	"testing"
)

func TestParamRangeValues(t *testing.T) {
	tests := []struct {
		name     string
		pr       ParamRange
		expected []float64
	}{
		{
			name:     "confluence 0.50-0.80 step 0.05",
			pr:       ParamRange{"confluence", 0.50, 0.80, 0.05},
			expected: []float64{0.50, 0.55, 0.60, 0.65, 0.70, 0.75, 0.80},
		},
		{
			name:     "ofi persistence 2-8 step 2",
			pr:       ParamRange{"ofiPersist", 2, 8, 2},
			expected: []float64{2, 4, 6, 8},
		},
		{
			name:     "stop ATR 1.0-2.5 step 0.5",
			pr:       ParamRange{"stopATR", 1.0, 2.5, 0.5},
			expected: []float64{1.0, 1.5, 2.0, 2.5},
		},
		{
			name:     "target ATR 2.0-4.0 step 0.5",
			pr:       ParamRange{"targetATR", 2.0, 4.0, 0.5},
			expected: []float64{2.0, 2.5, 3.0, 3.5, 4.0},
		},
		{
			name:     "max hold 1-4 step 1",
			pr:       ParamRange{"maxHold", 1, 4, 1},
			expected: []float64{1, 2, 3, 4},
		},
		{
			name:     "zero step returns min",
			pr:       ParamRange{"single", 5.0, 10.0, 0},
			expected: []float64{5.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.pr.Values()
			if len(got) != len(tt.expected) {
				t.Fatalf("len: got %d, want %d; values: %v", len(got), len(tt.expected), got)
			}
			for i, v := range got {
				if math.Abs(v-tt.expected[i]) > 1e-9 {
					t.Errorf("index %d: got %f, want %f", i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestDefaultParamGridCount(t *testing.T) {
	g := DefaultParamGrid()
	want := 2240
	got := g.Count()
	if got != want {
		t.Fatalf("Count: got %d, want %d", got, want)
	}
}

func TestEnumerateMatchesCount(t *testing.T) {
	g := DefaultParamGrid()
	configs := g.Enumerate()
	if len(configs) != g.Count() {
		t.Fatalf("Enumerate len %d != Count %d", len(configs), g.Count())
	}
}

func TestEnumerateConfigsValid(t *testing.T) {
	g := DefaultParamGrid()
	configs := g.Enumerate()

	for i, cfg := range configs {
		if cfg.Strategy.ConfluenceThreshold < 0.50 || cfg.Strategy.ConfluenceThreshold > 0.80 {
			t.Errorf("config %d: ConfluenceThreshold %f out of range", i, cfg.Strategy.ConfluenceThreshold)
		}
		if cfg.Strategy.MinOFIPersistence < 2 || cfg.Strategy.MinOFIPersistence > 8 {
			t.Errorf("config %d: MinOFIPersistence %d out of range", i, cfg.Strategy.MinOFIPersistence)
		}
		if cfg.Position.StopATRMult < 1.0 || cfg.Position.StopATRMult > 2.5 {
			t.Errorf("config %d: StopATRMult %f out of range", i, cfg.Position.StopATRMult)
		}
		if cfg.Position.TargetATRMult < 2.0 || cfg.Position.TargetATRMult > 4.0 {
			t.Errorf("config %d: TargetATRMult %f out of range", i, cfg.Position.TargetATRMult)
		}
		holdHours := float64(cfg.Position.MaxHoldingMs) / (60 * 60 * 1000)
		if holdHours < 1.0 || holdHours > 4.0 {
			t.Errorf("config %d: MaxHoldingHours %f out of range", i, holdHours)
		}
		if cfg.InitialEquity != 10000.0 {
			t.Errorf("config %d: InitialEquity %f, want 10000", i, cfg.InitialEquity)
		}
	}
}

func TestEnumerateUniqueness(t *testing.T) {
	// Use a small grid to verify all configs are unique
	g := ParamGrid{
		ConfluenceThreshold: ParamRange{"c", 0.5, 0.6, 0.1},
		MinOFIPersistence:   ParamRange{"o", 2, 4, 2},
		StopATRMult:         ParamRange{"s", 1.0, 1.5, 0.5},
		TargetATRMult:       ParamRange{"t", 2.0, 2.5, 0.5},
		MaxHoldingHours:     ParamRange{"h", 1, 2, 1},
	}

	configs := g.Enumerate()
	want := 2 * 2 * 2 * 2 * 2 // 32
	if len(configs) != want {
		t.Fatalf("small grid: got %d, want %d", len(configs), want)
	}

	// Check uniqueness via a simple key
	type key struct {
		ct   float64
		ofi  int
		stop float64
		tgt  float64
		hold int64
	}
	seen := make(map[key]bool)
	for _, cfg := range configs {
		k := key{
			ct:   cfg.Strategy.ConfluenceThreshold,
			ofi:  cfg.Strategy.MinOFIPersistence,
			stop: cfg.Position.StopATRMult,
			tgt:  cfg.Position.TargetATRMult,
			hold: cfg.Position.MaxHoldingMs,
		}
		if seen[k] {
			t.Errorf("duplicate config: %+v", k)
		}
		seen[k] = true
	}
}
