package walkforward

import (
	"fmt"
	"math"
	"sort"
)

// StabilityAnalysis measures parameter consistency across folds.
type StabilityAnalysis struct {
	ParamStats []ParamStability `json:"paramStats"`
	Verdict    string           `json:"verdict"` // "stable", "moderate", "unstable"
	Flags      []string         `json:"flags"`
}

// ParamStability holds statistics for a single parameter across folds.
type ParamStability struct {
	Name   string    `json:"name"`
	Mean   float64   `json:"mean"`
	StdDev float64   `json:"stddev"`
	CV     float64   `json:"cv"` // coefficient of variation (stddev/mean)
	Values []float64 `json:"values"`
}

// analyzeStability checks parameter consistency across folds.
// Extracts 5 params from each fold's BestConfig:
//   - ConfluenceThreshold, MinOFIPersistence (as float), StopATRMult, TargetATRMult, MaxHoldingMs (as hours)
//
// CV thresholds: stable < 0.15, moderate 0.15-0.30, unstable > 0.30
func analyzeStability(folds []FoldResult) StabilityAnalysis {
	if len(folds) <= 1 {
		// With 0 or 1 fold, we cannot determine instability.
		return StabilityAnalysis{
			Verdict: "stable",
			Flags:   []string{},
		}
	}

	type paramExtractor struct {
		name    string
		extract func(fr FoldResult) float64
	}

	extractors := []paramExtractor{
		{
			name: "ConfluenceThreshold",
			extract: func(fr FoldResult) float64 {
				return fr.BestConfig.Strategy.ConfluenceThreshold
			},
		},
		{
			name: "MinOFIPersistence",
			extract: func(fr FoldResult) float64 {
				return float64(fr.BestConfig.Strategy.MinOFIPersistence)
			},
		},
		{
			name: "StopATRMult",
			extract: func(fr FoldResult) float64 {
				return fr.BestConfig.Position.StopATRMult
			},
		},
		{
			name: "TargetATRMult",
			extract: func(fr FoldResult) float64 {
				return fr.BestConfig.Position.TargetATRMult
			},
		},
		{
			name: "MaxHoldingHours",
			extract: func(fr FoldResult) float64 {
				return float64(fr.BestConfig.Position.MaxHoldingMs) / (60 * 60 * 1000)
			},
		},
	}

	var stats []ParamStability
	worstCV := 0.0

	for _, ext := range extractors {
		values := make([]float64, len(folds))
		for i, fold := range folds {
			values[i] = ext.extract(fold)
		}

		mean, stddev := computeMeanStdDev(values)
		cv := 0.0
		if mean != 0 {
			cv = stddev / math.Abs(mean)
		}

		stats = append(stats, ParamStability{
			Name:   ext.name,
			Mean:   mean,
			StdDev: stddev,
			CV:     cv,
			Values: values,
		})

		if cv > worstCV {
			worstCV = cv
		}
	}

	// Determine verdict based on worst CV.
	verdict := classifyCV(worstCV)

	// Generate flags for parameters with notable variation.
	var flags []string
	for _, s := range stats {
		if s.CV > 0.20 {
			flags = append(flags, fmt.Sprintf("%s varies >20%% across folds (CV: %.0f%%)", s.Name, s.CV*100))
		}
	}

	return StabilityAnalysis{
		ParamStats: stats,
		Verdict:    verdict,
		Flags:      flags,
	}
}

// classifyCV returns the stability verdict for a given coefficient of variation.
func classifyCV(cv float64) string {
	switch {
	case cv < 0.15:
		return "stable"
	case cv <= 0.30:
		return "moderate"
	default:
		return "unstable"
	}
}

// computeMeanStdDev computes mean and population standard deviation of a slice.
func computeMeanStdDev(xs []float64) (mean, stddev float64) {
	n := float64(len(xs))
	if n == 0 {
		return 0, 0
	}
	for _, x := range xs {
		mean += x
	}
	mean /= n
	var variance float64
	for _, x := range xs {
		d := x - mean
		variance += d * d
	}
	variance /= n
	stddev = math.Sqrt(variance)
	return
}

// median returns the median of a sorted slice of float64 values.
// The input slice is sorted in place.
func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	return sorted[len(sorted)/2]
}
