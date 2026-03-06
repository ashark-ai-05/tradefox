package scanner

// DetectSwingPoints finds swing highs and lows in a candle series.
// A swing high: candle high > previous candle high AND candle high > next candle high
// A swing low: candle low < previous candle low AND candle low < next candle low
func DetectSwingPoints(candles []Candle) []SwingPoint {
	if len(candles) < 3 {
		return nil
	}

	var swings []SwingPoint
	total := len(candles)

	for i := 1; i < total-1; i++ {
		candlesAgo := total - 1 - i

		// Swing High
		if candles[i].High > candles[i-1].High && candles[i].High > candles[i+1].High {
			class := classifySwing(candles, i, total)
			swings = append(swings, SwingPoint{
				Type:       "SwingHigh",
				Class:      class,
				Price:      candles[i].High,
				Index:      i,
				CandlesAgo: candlesAgo,
			})
		}

		// Swing Low
		if candles[i].Low < candles[i-1].Low && candles[i].Low < candles[i+1].Low {
			class := classifySwing(candles, i, total)
			swings = append(swings, SwingPoint{
				Type:       "SwingLow",
				Class:      class,
				Price:      candles[i].Low,
				Index:      i,
				CandlesAgo: candlesAgo,
			})
		}
	}

	return swings
}

// classifySwing determines C1/C2/C3 based on confirmation candles after the swing.
func classifySwing(candles []Candle, swingIdx, total int) string {
	confirmations := total - 1 - swingIdx
	switch {
	case confirmations >= 3:
		return "C3"
	case confirmations == 2:
		return "C2"
	default:
		return "C1"
	}
}

// GetLatestSwing returns the most recent swing point.
func GetLatestSwing(candles []Candle) SwingResult {
	swings := DetectSwingPoints(candles)
	if len(swings) == 0 {
		return SwingResult{}
	}

	latest := swings[len(swings)-1]
	return SwingResult{
		Type:       latest.Type,
		Class:      latest.Class,
		Price:      latest.Price,
		CandlesAgo: latest.CandlesAgo,
	}
}
