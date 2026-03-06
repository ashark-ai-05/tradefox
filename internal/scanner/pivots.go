package scanner

import "math"

// CalcPivots computes standard pivot points from high, low, close.
func CalcPivots(high, low, close float64) PivotLevels {
	p := (high + low + close) / 3
	return PivotLevels{
		P:  p,
		S1: 2*p - high,
		R1: 2*p - low,
		S2: p - (high - low),
		R2: p + (high - low),
		S3: low - 2*(high-p),
		R3: high + 2*(p-low),
	}
}

// CalcWeeklyPivots computes weekly pivot levels.
func CalcWeeklyPivots(weekHigh, weekLow, weekClose float64) PivotLevels {
	return CalcPivots(weekHigh, weekLow, weekClose)
}

// CalcMonthlyPivots computes monthly pivot levels.
func CalcMonthlyPivots(monthHigh, monthLow, monthClose float64) PivotLevels {
	return CalcPivots(monthHigh, monthLow, monthClose)
}

// ClassifyPivotWidth categorizes the pivot range relative to current price.
func ClassifyPivotWidth(s1, r1, currentPrice float64) string {
	if currentPrice == 0 {
		return "Normal"
	}
	width := (r1 - s1) / currentPrice * 100
	switch {
	case width < 2:
		return "Narrow"
	case width > 5:
		return "Wide"
	default:
		return "Normal"
	}
}

// FindNearestPivot finds the nearest pivot level to the current price.
func FindNearestPivot(pivots PivotLevels, currentPrice float64) ProximityResult {
	levels := []struct {
		price float64
		label string
		typ   string
	}{
		{pivots.R3, "R3", "Resistance"},
		{pivots.R2, "R2", "Resistance"},
		{pivots.R1, "R1", "Resistance"},
		{pivots.P, "Pivot", "Support"},
		{pivots.S1, "S1", "Support"},
		{pivots.S2, "S2", "Support"},
		{pivots.S3, "S3", "Support"},
	}

	var nearest ProximityResult
	minDist := math.MaxFloat64

	for _, l := range levels {
		dist := math.Abs(currentPrice - l.price)
		if dist < minDist {
			minDist = dist
			pctDist := ((l.price - currentPrice) / currentPrice) * 100
			nearest = ProximityResult{
				Distance: pctDist,
				Type:     l.typ,
				Level:    l.label,
				Price:    l.price,
			}
		}
	}

	return nearest
}

// ExtractWeeklyHLC extracts the high, low, close from weekly candles (previous week).
func ExtractWeeklyHLC(weeklyCandles []Candle) (high, low, close float64) {
	if len(weeklyCandles) < 2 {
		return
	}
	// Use the previous completed week
	prev := weeklyCandles[len(weeklyCandles)-2]
	return prev.High, prev.Low, prev.Close
}

// ExtractMonthlyHLC extracts the high, low, close from monthly candles (previous month).
func ExtractMonthlyHLC(monthlyCandles []Candle) (high, low, close float64) {
	if len(monthlyCandles) < 2 {
		return
	}
	prev := monthlyCandles[len(monthlyCandles)-2]
	return prev.High, prev.Low, prev.Close
}
