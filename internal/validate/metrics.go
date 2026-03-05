package validate

import (
	"math"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/signals"
)

// SignalExtractor extracts a single float64 value from a SignalSet.
type SignalExtractor func(s *signals.SignalSet) float64

// AllExtractors returns named extractors for the 8 signals.
func AllExtractors() map[string]SignalExtractor {
	return map[string]SignalExtractor{
		"microprice": func(s *signals.SignalSet) float64 { return s.Microprice.DivBps },
		"ofi":        func(s *signals.SignalSet) float64 { return s.OFI.Value },
		"depth":      func(s *signals.SignalSet) float64 { return s.DepthImb.Weighted },
		"sweep": func(s *signals.SignalSet) float64 {
			if s.Sweep.Active {
				if s.Sweep.Dir == "buy" {
					return 1.0
				}
				return -1.0
			}
			return 0.0
		},
		"lambda":    func(s *signals.SignalSet) float64 { return s.Lambda.Value },
		"vol":       func(s *signals.SignalSet) float64 { return s.Vol.Realized },
		"spoof":     func(s *signals.SignalSet) float64 { return s.Spoof.Score },
		"composite": func(s *signals.SignalSet) float64 { return s.Composite.Avg },
	}
}

// Correlation computes Pearson correlation between signal values and forward returns at a given horizon.
// Returns NaN if insufficient data or zero variance.
func Correlation(rows []ReturnRow, extract SignalExtractor, horizon time.Duration) float64 {
	var xs, ys []float64
	for _, row := range rows {
		ret, ok := row.Returns[horizon]
		if !ok || math.IsNaN(ret) {
			continue
		}
		xs = append(xs, extract(row.Signals))
		ys = append(ys, ret)
	}

	if len(xs) < 30 {
		return math.NaN()
	}

	return pearson(xs, ys)
}

// HitRate computes the fraction of times the signal correctly predicts return direction
// when |signal| exceeds the threshold.
func HitRate(rows []ReturnRow, extract SignalExtractor, threshold float64, horizon time.Duration) (rate float64, n int) {
	var correct, total int
	for _, row := range rows {
		sig := extract(row.Signals)
		if math.Abs(sig) < threshold {
			continue
		}
		ret, ok := row.Returns[horizon]
		if !ok || math.IsNaN(ret) {
			continue
		}
		total++
		// Signal correctly predicted direction?
		if (sig > 0 && ret > 0) || (sig < 0 && ret < 0) {
			correct++
		}
	}
	if total == 0 {
		return 0, 0
	}
	return float64(correct) / float64(total), total
}

// DecayCurve computes correlation at each horizon for a signal.
func DecayCurve(rows []ReturnRow, extract SignalExtractor, horizons []time.Duration) map[time.Duration]float64 {
	curve := make(map[time.Duration]float64, len(horizons))
	for _, h := range horizons {
		curve[h] = Correlation(rows, extract, h)
	}
	return curve
}

// pearson computes Pearson correlation coefficient.
func pearson(xs, ys []float64) float64 {
	n := float64(len(xs))
	if n < 2 {
		return math.NaN()
	}

	var sumX, sumY, sumXY, sumX2, sumY2 float64
	for i := range xs {
		sumX += xs[i]
		sumY += ys[i]
		sumXY += xs[i] * ys[i]
		sumX2 += xs[i] * xs[i]
		sumY2 += ys[i] * ys[i]
	}

	num := n*sumXY - sumX*sumY
	den := math.Sqrt((n*sumX2 - sumX*sumX) * (n*sumY2 - sumY*sumY))
	if den == 0 {
		return math.NaN()
	}
	return num / den
}
