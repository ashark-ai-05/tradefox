package walkforward

import (
	"math"

	"github.com/ashark-ai-05/tradefox/internal/backtest"
)

// ParamRange defines a range for a single parameter.
type ParamRange struct {
	Name string
	Min  float64
	Max  float64
	Step float64
}

// Values returns all discrete values in the range (inclusive).
func (p ParamRange) Values() []float64 {
	if p.Step <= 0 {
		return []float64{p.Min}
	}
	if p.Max < p.Min {
		return nil
	}
	n := int(math.Round((p.Max-p.Min)/p.Step)) + 1
	vals := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		v := p.Min + float64(i)*p.Step
		// Round to avoid floating point drift.
		v = math.Round(v*1e9) / 1e9
		if v > p.Max+p.Step*0.01 {
			break
		}
		vals = append(vals, v)
	}
	return vals
}

// ParamGrid defines the parameter search space.
type ParamGrid struct {
	ConfluenceThreshold ParamRange // 0.50-0.80, step 0.05 (7 values)
	MinOFIPersistence   ParamRange // 2-8, step 2 (4 values)
	StopATRMult         ParamRange // 1.0-2.5, step 0.5 (4 values)
	TargetATRMult       ParamRange // 2.0-4.0, step 0.5 (5 values)
	MaxHoldingHours     ParamRange // 1-4, step 1 (4 values)
}

// DefaultParamGrid returns the default grid. Total: 7*4*4*5*4 = 2240 configs.
func DefaultParamGrid() ParamGrid {
	return ParamGrid{
		ConfluenceThreshold: ParamRange{"confluence", 0.50, 0.80, 0.05},
		MinOFIPersistence:   ParamRange{"ofiPersist", 2, 8, 2},
		StopATRMult:         ParamRange{"stopATR", 1.0, 2.5, 0.5},
		TargetATRMult:       ParamRange{"targetATR", 2.0, 4.0, 0.5},
		MaxHoldingHours:     ParamRange{"maxHold", 1, 4, 1},
	}
}

// Count returns the total number of combinations.
func (g ParamGrid) Count() int {
	return len(g.ConfluenceThreshold.Values()) *
		len(g.MinOFIPersistence.Values()) *
		len(g.StopATRMult.Values()) *
		len(g.TargetATRMult.Values()) *
		len(g.MaxHoldingHours.Values())
}

// Enumerate generates all EngineConfig combinations.
// Starts from backtest.DefaultEngineConfig() and overrides the grid params.
func (g ParamGrid) Enumerate() []backtest.EngineConfig {
	base := backtest.DefaultEngineConfig()
	total := g.Count()
	configs := make([]backtest.EngineConfig, 0, total)

	for _, ct := range g.ConfluenceThreshold.Values() {
		for _, ofi := range g.MinOFIPersistence.Values() {
			for _, stop := range g.StopATRMult.Values() {
				for _, target := range g.TargetATRMult.Values() {
					for _, hold := range g.MaxHoldingHours.Values() {
						cfg := base
						cfg.Strategy.ConfluenceThreshold = ct
						cfg.Strategy.MinOFIPersistence = int(ofi)
						cfg.Position.StopATRMult = stop
						cfg.Position.TargetATRMult = target
						cfg.Position.MaxHoldingMs = int64(hold * 60 * 60 * 1000)
						configs = append(configs, cfg)
					}
				}
			}
		}
	}
	return configs
}
