package scanner

import "math"

// DetectFVGs finds Fair Value Gaps in a candle series.
// Bullish FVG: candle[i-2].High < candle[i].Low (gap up)
// Bearish FVG: candle[i-2].Low > candle[i].High (gap down)
func DetectFVGs(candles []Candle, timeframe string) []FVG {
	if len(candles) < 3 {
		return nil
	}

	var fvgs []FVG
	currentPrice := candles[len(candles)-1].Close

	for i := 2; i < len(candles); i++ {
		// Bullish FVG: gap between candle[i-2] high and candle[i] low
		if candles[i-2].High < candles[i].Low {
			fvg := FVG{
				High:      candles[i].Low,
				Low:       candles[i-2].High,
				Type:      "Bullish",
				Index:     i,
				Timeframe: timeframe,
			}
			// Check fill status
			fvg.FillPct, fvg.Filled = calcFVGFill(fvg, candles[i:], currentPrice)
			fvgs = append(fvgs, fvg)
		}

		// Bearish FVG: gap between candle[i] high and candle[i-2] low
		if candles[i-2].Low > candles[i].High {
			fvg := FVG{
				High:      candles[i-2].Low,
				Low:       candles[i].High,
				Type:      "Bearish",
				Index:     i,
				Timeframe: timeframe,
			}
			fvg.FillPct, fvg.Filled = calcFVGFill(fvg, candles[i:], currentPrice)
			fvgs = append(fvgs, fvg)
		}
	}

	return fvgs
}

// calcFVGFill determines how much of an FVG has been filled by subsequent price action.
func calcFVGFill(fvg FVG, subsequentCandles []Candle, currentPrice float64) (float64, bool) {
	gapSize := fvg.High - fvg.Low
	if gapSize <= 0 {
		return 100, true
	}

	var maxPenetration float64

	for _, c := range subsequentCandles {
		if fvg.Type == "Bullish" {
			// Price needs to retrace down into the gap
			if c.Low < fvg.High {
				penetration := fvg.High - c.Low
				if penetration > maxPenetration {
					maxPenetration = penetration
				}
			}
		} else {
			// Price needs to retrace up into the gap
			if c.High > fvg.Low {
				penetration := c.High - fvg.Low
				if penetration > maxPenetration {
					maxPenetration = penetration
				}
			}
		}
	}

	fillPct := (maxPenetration / gapSize) * 100
	if fillPct > 100 {
		fillPct = 100
	}

	return fillPct, fillPct >= 100
}

// FindNearestFVG finds the closest unfilled FVG and computes proximity from current price.
func FindNearestFVG(fvgs []FVG, currentPrice float64) FVGResult {
	if len(fvgs) == 0 || currentPrice == 0 {
		return FVGResult{}
	}

	var nearest *FVG
	minDist := math.MaxFloat64

	for i := range fvgs {
		if fvgs[i].Filled {
			continue
		}

		// Distance to the midpoint of the FVG
		mid := (fvgs[i].High + fvgs[i].Low) / 2
		dist := math.Abs(currentPrice - mid)
		if dist < minDist {
			minDist = dist
			nearest = &fvgs[i]
		}
	}

	if nearest == nil {
		return FVGResult{}
	}

	mid := (nearest.High + nearest.Low) / 2
	proximity := ((mid - currentPrice) / currentPrice) * 100

	return FVGResult{
		Proximity: proximity,
		Type:      nearest.Type,
		Timeframe: nearest.Timeframe,
		Level:     mid,
		FillPct:   nearest.FillPct,
	}
}
