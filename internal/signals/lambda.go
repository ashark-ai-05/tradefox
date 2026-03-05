package signals

import "math"

// ComputeLambda estimates Kyle's Lambda via linear regression of
// price changes (bps) vs signed volume over windows of 5 trades.
// Requires at least 20 trades.
func ComputeLambda(trades []TradeRecord, prevSmoothed float64) LambdaSignal {
	if len(trades) < 20 {
		return LambdaSignal{Value: prevSmoothed, Regime: lambdaRegime(prevSmoothed)}
	}

	start := len(trades) - 100
	if start < 0 {
		start = 0
	}
	r := trades[start:]

	const W = 5
	var dps, svs []float64

	for i := W; i < len(r); i += W {
		pe := r[i].Price
		ps := r[i-W].Price
		if pe == 0 || ps == 0 {
			continue
		}
		dp := ((pe - ps) / ps) * 10000
		dps = append(dps, dp)

		sv := 0.0
		for j := i - W; j < i; j++ {
			if r[j].IsBuy {
				sv += r[j].Size
			} else {
				sv -= r[j].Size
			}
		}
		svs = append(svs, sv)
	}

	rawLambda := 0.0
	if len(dps) >= 3 {
		n := float64(len(dps))
		var sx, sy, sxy, sxx float64
		for i := range dps {
			sx += svs[i]
			sy += dps[i]
			sxy += svs[i] * dps[i]
			sxx += svs[i] * svs[i]
		}
		den := n*sxx - sx*sx
		if math.Abs(den) > 1e-10 {
			rawLambda = math.Abs((n*sxy - sx*sy) / den)
		} else if math.Abs(sx) > 1e-10 {
			// Signed volume is constant; fall back to mean price-impact ratio.
			rawLambda = math.Abs(sy / sx)
		}
	}

	val := EMA(prevSmoothed, rawLambda)
	return LambdaSignal{Value: val, Regime: lambdaRegime(val)}
}

func lambdaRegime(v float64) string {
	if v > 2.0 {
		return "high"
	}
	if v > 0.5 {
		return "medium"
	}
	return "low"
}
