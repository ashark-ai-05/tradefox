package scanner

// CalcBias analyzes candles for swing structure to determine directional bias.
func CalcBias(candles []Candle) BiasResult {
	if len(candles) < 10 {
		return BiasResult{Direction: "None", Tag: ""}
	}

	// Find the last several swing highs and lows
	swings := DetectSwingPoints(candles)
	if len(swings) < 4 {
		return BiasResult{Direction: "None", Tag: ""}
	}

	// Get recent swing highs and lows
	var recentHighs, recentLows []SwingPoint
	for _, s := range swings {
		if s.Type == "SwingHigh" {
			recentHighs = append(recentHighs, s)
		} else {
			recentLows = append(recentLows, s)
		}
	}

	if len(recentHighs) < 2 || len(recentLows) < 2 {
		return BiasResult{Direction: "None", Tag: ""}
	}

	// Compare last two swing highs and lows
	lastH := recentHighs[len(recentHighs)-1]
	prevH := recentHighs[len(recentHighs)-2]
	lastL := recentLows[len(recentLows)-1]
	prevL := recentLows[len(recentLows)-2]

	higherHighs := lastH.Price > prevH.Price
	higherLows := lastL.Price > prevL.Price
	lowerHighs := lastH.Price < prevH.Price
	lowerLows := lastL.Price < prevL.Price

	currentPrice := candles[len(candles)-1].Close

	var direction string
	switch {
	case higherHighs && higherLows:
		direction = "High"
	case lowerHighs && lowerLows:
		direction = "Low"
	default:
		return BiasResult{Direction: "None", Tag: ""}
	}

	// Determine tag: C (continuation) or R (reversal)
	tag := "C"
	if direction == "High" && currentPrice < lastL.Price {
		tag = "R" // Price reversing below last swing low in uptrend
	} else if direction == "Low" && currentPrice > lastH.Price {
		tag = "R" // Price reversing above last swing high in downtrend
	}

	return BiasResult{Direction: direction, Tag: tag}
}
