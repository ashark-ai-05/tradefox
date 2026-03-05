package validate

import (
	"time"
)

// RegimeResult holds metrics split by volatility regime.
type RegimeResult struct {
	Signal  string        `json:"signal"`
	Regime  string        `json:"regime"` // "low", "medium", "high", "extreme"
	Horizon time.Duration `json:"horizon"`
	Corr    float64       `json:"corr"`
	HitRate float64       `json:"hitRate"`
	N       int           `json:"n"`
}

// ComputeRegimeMetrics splits rows by vol regime and computes metrics per regime.
func ComputeRegimeMetrics(rows []ReturnRow, signalName string, extract SignalExtractor, horizons []time.Duration) []RegimeResult {
	// Group rows by vol regime
	regimeRows := map[string][]ReturnRow{}
	for _, row := range rows {
		regime := row.Signals.Vol.Regime
		if regime == "" {
			regime = "unknown"
		}
		regimeRows[regime] = append(regimeRows[regime], row)
	}

	var results []RegimeResult
	for regime, rRows := range regimeRows {
		for _, h := range horizons {
			corr := Correlation(rRows, extract, h)
			hr, n := HitRate(rRows, extract, 0.10, h) // use a moderate threshold

			results = append(results, RegimeResult{
				Signal:  signalName,
				Regime:  regime,
				Horizon: h,
				Corr:    corr,
				HitRate: hr,
				N:       n,
			})
		}
	}

	return results
}
