package validate

import (
	"math"
	"time"
)

// InteractionResult measures whether combining two signals improves prediction.
type InteractionResult struct {
	Signal1    string        `json:"signal1"`
	Signal2    string        `json:"signal2"`
	Threshold1 float64       `json:"threshold1"`
	Threshold2 float64       `json:"threshold2"`
	Horizon    time.Duration `json:"horizon"`
	HitRate    float64       `json:"hitRate"`
	N          int           `json:"n"`
	Lift       float64       `json:"lift"` // HitRate_combined - max(HitRate_individual)
}

// ComputeInteractions tests key signal pairs from the strategy doc.
func ComputeInteractions(rows []ReturnRow) []InteractionResult {
	extractors := AllExtractors()
	horizon := 60 * time.Second // primary horizon for interaction testing

	pairs := []struct {
		name1, name2     string
		thresh1, thresh2 float64
	}{
		{"ofi", "microprice", 0.15, 0.5},
		{"ofi", "depth", 0.15, 0.20},
		{"sweep", "ofi", 0, 0.10},     // sweep active AND OFI aligned
		{"composite", "vol", 0.20, 0}, // vol: check regime, not threshold
	}

	var results []InteractionResult
	for _, pair := range pairs {
		ext1 := extractors[pair.name1]
		ext2 := extractors[pair.name2]

		// Individual hit rates
		hr1, _ := HitRate(rows, ext1, pair.thresh1, horizon)
		hr2, _ := HitRate(rows, ext2, pair.thresh2, horizon)

		// Combined hit rate
		var correct, total int
		for _, row := range rows {
			v1 := ext1(row.Signals)
			v2 := ext2(row.Signals)
			if math.Abs(v1) < pair.thresh1 || math.Abs(v2) < pair.thresh2 {
				continue
			}
			ret, ok := row.Returns[horizon]
			if !ok || math.IsNaN(ret) {
				continue
			}
			total++
			// Both signals agree on direction and return is in that direction
			if (v1 > 0 && v2 > 0 && ret > 0) || (v1 < 0 && v2 < 0 && ret < 0) {
				correct++
			}
		}

		var combined float64
		if total > 0 {
			combined = float64(correct) / float64(total)
		}

		lift := combined - math.Max(hr1, hr2)

		results = append(results, InteractionResult{
			Signal1:    pair.name1,
			Signal2:    pair.name2,
			Threshold1: pair.thresh1,
			Threshold2: pair.thresh2,
			Horizon:    horizon,
			HitRate:    combined,
			N:          total,
			Lift:       lift,
		})
	}

	return results
}
