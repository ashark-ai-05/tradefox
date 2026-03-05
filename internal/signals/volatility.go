package signals

import "math"

// VolState holds rolling volatility history for trend detection.
type VolState struct {
	History []float64
}

// ComputeVolatility computes realized volatility from trade log returns.
func ComputeVolatility(trades []TradeRecord, prevSmoothed float64, state *VolState) VolSignal {
	if len(trades) < 20 {
		return VolSignal{Realized: prevSmoothed, Regime: volRegime(prevSmoothed), Trend: "stable"}
	}

	start := len(trades) - 100
	if start < 0 {
		start = 0
	}
	r := trades[start:]

	var rets []float64
	for i := 1; i < len(r); i++ {
		p0 := r[i-1].Price
		p1 := r[i].Price
		if p0 > 0 && p1 > 0 {
			rets = append(rets, math.Log(p1/p0))
		}
	}

	rawVol := 0.0
	if len(rets) >= 5 {
		ssq := 0.0
		for _, ret := range rets {
			ssq += ret * ret
		}
		rawVol = math.Sqrt((ssq/float64(len(rets)))*365.25*24*3600) * 100
	}

	smoothed := EMA(prevSmoothed, rawVol)

	state.History = append(state.History, smoothed)
	if len(state.History) > 20 {
		state.History = state.History[len(state.History)-20:]
	}

	trend := "stable"
	if len(state.History) >= 5 {
		recent := avg(state.History[len(state.History)-3:])
		old := avg(state.History[:3])
		if old > 0 {
			if recent > old*1.3 {
				trend = "rising"
			} else if recent < old*0.7 {
				trend = "falling"
			}
		}
	}

	return VolSignal{
		Realized: smoothed,
		Regime:   volRegime(smoothed),
		Trend:    trend,
	}
}

func volRegime(v float64) string {
	switch {
	case v > 150:
		return "extreme"
	case v > 80:
		return "high"
	case v > 30:
		return "normal"
	default:
		return "low"
	}
}

func avg(s []float64) float64 {
	if len(s) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range s {
		sum += v
	}
	return sum / float64(len(s))
}
