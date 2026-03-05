package signals

import "math"

// ComputeDepthImbalance computes exponentially-weighted depth imbalance
// across up to 10 levels. Decay factor = 0.3 per level.
func ComputeDepthImbalance(bids, asks []BookLevel, prevWeighted float64) DepthImbSignal {
	n := len(bids)
	if len(asks) < n {
		n = len(asks)
	}
	if n > 10 {
		n = 10
	}

	levels := make([]float64, n)
	wSum := 0.0
	wTot := 0.0

	for i := 0; i < n; i++ {
		bs := bids[i].Size
		as := asks[i].Size
		tot := bs + as
		imb := 0.0
		if tot > 0 {
			imb = (bs - as) / tot
		}
		levels[i] = imb

		w := math.Exp(-float64(i) * 0.3)
		wSum += imb * w
		wTot += w
	}

	raw := 0.0
	if wTot > 0 {
		raw = wSum / wTot
	}

	weighted := EMA(prevWeighted, raw)

	pressure := "neutral"
	if weighted > 0.15 {
		pressure = "bid"
	} else if weighted < -0.15 {
		pressure = "ask"
	}

	return DepthImbSignal{
		Levels:   levels,
		Weighted: weighted,
		Pressure: pressure,
	}
}
