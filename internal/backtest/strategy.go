package backtest

import (
	"math"
)

// StrategyConfig holds confluence and entry parameters.
type StrategyConfig struct {
	// Weights for B1-B9 components
	Weights [9]float64

	// Entry thresholds
	ConfluenceThreshold float64 // default: 0.60
	MinOFIPersistence   int     // default: 3 consecutive aligned readings

	// Vol regime veto
	VetoExtreme bool // default: true -- no new entries in "extreme" vol
}

// DefaultStrategyConfig returns the default strategy parameters.
func DefaultStrategyConfig() StrategyConfig {
	return StrategyConfig{
		Weights: [9]float64{
			0.20, // B1: OFI
			0.15, // B2: Microprice
			0.10, // B3: Depth imbalance
			0.15, // B4: Sweep
			0.05, // B5: Lambda
			0.10, // B6: Vol regime
			0.05, // B7: Spoof
			0.10, // B8: Composite
			0.10, // B9: OFI persistence
		},
		ConfluenceThreshold: 0.60,
		MinOFIPersistence:   3,
		VetoExtreme:         true,
	}
}

// ConfluenceResult is the output of confluence scoring.
type ConfluenceResult struct {
	Score      float64
	Direction  int // +1 long, -1 short, 0 no signal
	Components [9]float64
	Vetoed     bool
	VetoReason string
}

// OFITracker tracks OFI direction persistence per symbol.
type OFITracker struct {
	counts  map[string]int // symbol -> consecutive aligned count
	lastDir map[string]int // symbol -> last direction
}

func NewOFITracker() *OFITracker {
	return &OFITracker{
		counts:  make(map[string]int),
		lastDir: make(map[string]int),
	}
}

// Update tracks OFI persistence and returns the consecutive count.
func (t *OFITracker) Update(symbol string, ofiValue float64) int {
	dir := 0
	if ofiValue > 0.05 {
		dir = 1
	}
	if ofiValue < -0.05 {
		dir = -1
	}

	if dir == t.lastDir[symbol] && dir != 0 {
		t.counts[symbol]++
	} else {
		t.counts[symbol] = 1
	}
	t.lastDir[symbol] = dir
	return t.counts[symbol]
}

// ComputeConfluence evaluates the 9-component confluence score.
func ComputeConfluence(cfg StrategyConfig, evt SignalEvent, ofiPersistence int) ConfluenceResult {
	var comps [9]float64
	sig := evt.Signals

	// B1: OFI direction + magnitude [-1, 1]
	comps[0] = clamp(sig.OFI.Value, -1, 1)

	// B2: Microprice divergence sign + magnitude [-1, 1]
	comps[1] = clamp(sig.Microprice.DivBps/2.0, -1, 1) // normalize: 2bps = full signal

	// B3: Depth imbalance weighted [-1, 1]
	comps[2] = clamp(sig.DepthImb.Weighted, -1, 1)

	// B4: Sweep (active + aligned)
	if sig.Sweep.Active {
		if sig.Sweep.Dir == "buy" {
			comps[3] = 1.0
		}
		if sig.Sweep.Dir == "sell" {
			comps[3] = -1.0
		}
	}

	// B5: Lambda regime (low lambda = favorable for trend-following)
	switch sig.Lambda.Regime {
	case "low":
		comps[4] = 0.5 // favorable
	case "medium":
		comps[4] = 0.0 // neutral
	case "high":
		comps[4] = -0.5 // unfavorable
	}

	// B6: Vol regime (low/medium = favorable)
	switch sig.Vol.Regime {
	case "low":
		comps[5] = 0.5
	case "medium":
		comps[5] = 0.25
	case "high":
		comps[5] = -0.25
	case "extreme":
		comps[5] = -0.5
	}

	// B7: Spoof (aligned with inferred direction)
	if sig.Spoof.Active {
		// Spoof on bid side suggests sell pressure -> bearish
		if sig.Spoof.Side == "bid" {
			comps[6] = -0.5
		}
		if sig.Spoof.Side == "ask" {
			comps[6] = 0.5
		}
	}

	// B8: Composite direction
	comps[7] = clamp(sig.Composite.Avg, -1, 1)

	// B9: OFI persistence (consecutive aligned readings)
	if ofiPersistence >= cfg.MinOFIPersistence {
		if sig.OFI.Value > 0 {
			comps[8] = 1.0
		}
		if sig.OFI.Value < 0 {
			comps[8] = -1.0
		}
	}

	// Weighted sum
	var longScore, shortScore float64
	for i := 0; i < 9; i++ {
		if comps[i] > 0 {
			longScore += comps[i] * cfg.Weights[i]
		} else if comps[i] < 0 {
			shortScore += math.Abs(comps[i]) * cfg.Weights[i]
		}
	}

	score := math.Max(longScore, shortScore)
	direction := 0
	if longScore > shortScore {
		direction = 1
	}
	if shortScore > longScore {
		direction = -1
	}

	result := ConfluenceResult{
		Score:      score,
		Direction:  direction,
		Components: comps,
	}

	// Vol regime veto
	if cfg.VetoExtreme && sig.Vol.Regime == "extreme" {
		result.Vetoed = true
		result.VetoReason = "extreme volatility regime"
	}

	return result
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
