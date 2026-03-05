package signals

import "math"

// ComputeComposite combines directional signals into a single aggregate.
func ComputeComposite(micro MicropriceSignal, ofi OFISignal, depth DepthImbSignal, sweep SweepSignal, prevSmoothed float64) CompositeSignal {
	sigs := []float64{
		dirToFloat(micro.Dir, "up", "down"),
		ofi.Value,
		depth.Weighted,
	}
	if sweep.Active {
		sigs = append(sigs, dirToFloat(sweep.Dir, "buy", "sell"))
	}

	sum := 0.0
	for _, s := range sigs {
		sum += s
	}
	rawAvg := sum / float64(len(sigs))

	smoothed := EMA(prevSmoothed, rawAvg)

	dir := "NEUTRAL"
	if smoothed > 0.2 {
		dir = "BULLISH"
	} else if smoothed < -0.2 {
		dir = "BEARISH"
	}

	strength := math.Abs(smoothed) * 100
	if strength > 100 {
		strength = 100
	}

	return CompositeSignal{
		Avg:      smoothed,
		Dir:      dir,
		Strength: strength,
	}
}

func dirToFloat(dir, positive, negative string) float64 {
	switch dir {
	case positive:
		return 1
	case negative:
		return -1
	default:
		return 0
	}
}
