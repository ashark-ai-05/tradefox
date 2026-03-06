package scanner

// DetectVolumeAnomaly analyzes 1H candles for volume anomalies using a 20-period moving average.
func DetectVolumeAnomaly(candles []Candle) VolumeAnomaly {
	if len(candles) == 0 {
		return VolumeAnomaly{State: "Normal"}
	}

	currentVol := candles[len(candles)-1].Volume

	// Calculate 20-period average (excluding current candle)
	lookback := 20
	start := len(candles) - 1 - lookback
	if start < 0 {
		start = 0
	}
	end := len(candles) - 1

	if end <= start {
		return VolumeAnomaly{CurrentVol: currentVol, AvgVol: currentVol, Ratio: 1, State: "Normal"}
	}

	var sum float64
	count := 0
	for i := start; i < end; i++ {
		sum += candles[i].Volume
		count++
	}

	if count == 0 || sum == 0 {
		return VolumeAnomaly{CurrentVol: currentVol, State: "Normal"}
	}

	avgVol := sum / float64(count)
	ratio := currentVol / avgVol

	state := classifyVolumeRatio(ratio)

	return VolumeAnomaly{
		CurrentVol: currentVol,
		AvgVol:     avgVol,
		Ratio:      ratio,
		State:      state,
	}
}

func classifyVolumeRatio(ratio float64) string {
	switch {
	case ratio > 3:
		return "Spike"
	case ratio > 2:
		return "Unusual"
	case ratio > 1.5:
		return "Elevated"
	default:
		return "Normal"
	}
}
