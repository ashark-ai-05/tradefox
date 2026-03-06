package walkforward

import (
	"math"
	"testing"

	"github.com/ashark-ai-05/tradefox/internal/backtest"
)

func makeFoldResult(confluence float64, ofi int, stop, target float64, holdHours int) FoldResult {
	cfg := backtest.DefaultEngineConfig()
	cfg.Strategy.ConfluenceThreshold = confluence
	cfg.Strategy.MinOFIPersistence = ofi
	cfg.Position.StopATRMult = stop
	cfg.Position.TargetATRMult = target
	cfg.Position.MaxHoldingMs = int64(holdHours) * 60 * 60 * 1000
	return FoldResult{
		BestConfig: cfg,
	}
}

func TestStabilityStableParams(t *testing.T) {
	// All folds have the same or very close parameters -> "stable".
	folds := []FoldResult{
		makeFoldResult(0.60, 3, 1.5, 3.0, 4),
		makeFoldResult(0.60, 3, 1.5, 3.0, 4),
		makeFoldResult(0.60, 3, 1.5, 3.0, 4),
	}

	result := analyzeStability(folds)

	if result.Verdict != "stable" {
		t.Errorf("expected verdict 'stable', got %q", result.Verdict)
	}

	// All CVs should be 0 since values are identical.
	for _, ps := range result.ParamStats {
		if ps.CV != 0 {
			t.Errorf("param %s: expected CV 0, got %.4f", ps.Name, ps.CV)
		}
	}

	if len(result.Flags) != 0 {
		t.Errorf("expected no flags, got %v", result.Flags)
	}
}

func TestStabilityStableSmallVariation(t *testing.T) {
	// Small variation should still be stable (CV < 0.15).
	folds := []FoldResult{
		makeFoldResult(0.60, 3, 1.5, 3.0, 4),
		makeFoldResult(0.62, 3, 1.5, 3.1, 4),
		makeFoldResult(0.61, 3, 1.5, 3.0, 4),
	}

	result := analyzeStability(folds)

	if result.Verdict != "stable" {
		t.Errorf("expected verdict 'stable', got %q", result.Verdict)
	}
}

func TestStabilityUnstableParams(t *testing.T) {
	// Widely varying parameters -> "unstable".
	folds := []FoldResult{
		makeFoldResult(0.50, 2, 1.0, 2.0, 1),
		makeFoldResult(0.80, 8, 2.5, 4.0, 4),
		makeFoldResult(0.50, 2, 1.0, 2.0, 1),
	}

	result := analyzeStability(folds)

	if result.Verdict != "unstable" {
		t.Errorf("expected verdict 'unstable', got %q", result.Verdict)
	}

	if len(result.Flags) == 0 {
		t.Error("expected flags for unstable params, got none")
	}
}

func TestStabilityModerateParams(t *testing.T) {
	// Moderate variation -> worst CV between 0.15-0.30.
	// StopATRMult values: 1.0, 1.5, 1.5 -> mean=1.333, stddev=0.236, CV=0.177
	// Other params are stable (identical).
	folds := []FoldResult{
		makeFoldResult(0.60, 3, 1.0, 3.0, 4),
		makeFoldResult(0.60, 3, 1.5, 3.0, 4),
		makeFoldResult(0.60, 3, 1.5, 3.0, 4),
	}

	result := analyzeStability(folds)

	if result.Verdict != "moderate" {
		t.Errorf("expected verdict 'moderate', got %q", result.Verdict)
	}
}

func TestStabilitySingleFold(t *testing.T) {
	// Single fold -> "stable" (not enough data to determine instability).
	folds := []FoldResult{
		makeFoldResult(0.60, 3, 1.5, 3.0, 4),
	}

	result := analyzeStability(folds)

	if result.Verdict != "stable" {
		t.Errorf("expected verdict 'stable' for single fold, got %q", result.Verdict)
	}
}

func TestStabilityEmptyFolds(t *testing.T) {
	result := analyzeStability(nil)

	if result.Verdict != "stable" {
		t.Errorf("expected verdict 'stable' for empty folds, got %q", result.Verdict)
	}
}

func TestComputeMeanStdDev(t *testing.T) {
	tests := []struct {
		name      string
		values    []float64
		wantMean  float64
		wantStdev float64
	}{
		{"identical", []float64{5, 5, 5}, 5, 0},
		{"varied", []float64{2, 4, 4, 4, 5, 5, 7, 9}, 5, 2},
		{"empty", nil, 0, 0},
		{"single", []float64{42}, 42, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mean, stddev := computeMeanStdDev(tt.values)
			if math.Abs(mean-tt.wantMean) > 0.01 {
				t.Errorf("mean: got %.4f, want %.4f", mean, tt.wantMean)
			}
			if math.Abs(stddev-tt.wantStdev) > 0.01 {
				t.Errorf("stddev: got %.4f, want %.4f", stddev, tt.wantStdev)
			}
		})
	}
}

func TestMedian(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		want   float64
	}{
		{"odd", []float64{3, 1, 2}, 2},
		{"even", []float64{4, 1, 3, 2}, 3},
		{"single", []float64{7}, 7},
		{"empty", nil, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := median(tt.values)
			if got != tt.want {
				t.Errorf("median(%v) = %.1f, want %.1f", tt.values, got, tt.want)
			}
		})
	}
}

func TestClassifyCV(t *testing.T) {
	tests := []struct {
		cv   float64
		want string
	}{
		{0.0, "stable"},
		{0.10, "stable"},
		{0.14, "stable"},
		{0.15, "moderate"},
		{0.25, "moderate"},
		{0.30, "moderate"},
		{0.31, "unstable"},
		{0.50, "unstable"},
		{1.0, "unstable"},
	}

	for _, tt := range tests {
		got := classifyCV(tt.cv)
		if got != tt.want {
			t.Errorf("classifyCV(%.2f) = %q, want %q", tt.cv, got, tt.want)
		}
	}
}
