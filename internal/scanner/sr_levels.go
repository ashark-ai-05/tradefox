package scanner

import "math"

// CalcSRLevels computes support/resistance levels from daily, weekly, and monthly candles.
func CalcSRLevels(dailyCandles, weeklyCandles, monthlyCandles []Candle) []SRLevel {
	var levels []SRLevel

	// Previous Day High/Low
	if len(dailyCandles) >= 2 {
		prev := dailyCandles[len(dailyCandles)-2]
		levels = append(levels,
			SRLevel{Price: prev.High, Type: "Resistance", Label: "PDH"},
			SRLevel{Price: prev.Low, Type: "Support", Label: "PDL"},
		)
	}

	// Previous Week High/Low
	if len(weeklyCandles) >= 2 {
		prev := weeklyCandles[len(weeklyCandles)-2]
		levels = append(levels,
			SRLevel{Price: prev.High, Type: "Resistance", Label: "PWH"},
			SRLevel{Price: prev.Low, Type: "Support", Label: "PWL"},
		)
	}

	// Monthly pivot levels
	if len(monthlyCandles) >= 2 {
		mh, ml, mc := ExtractMonthlyHLC(monthlyCandles)
		pivots := CalcMonthlyPivots(mh, ml, mc)
		levels = append(levels,
			SRLevel{Price: pivots.R3, Type: "Resistance", Label: "Monthly R3"},
			SRLevel{Price: pivots.R2, Type: "Resistance", Label: "Monthly R2"},
			SRLevel{Price: pivots.R1, Type: "Resistance", Label: "Monthly R1"},
			SRLevel{Price: pivots.P, Type: "Support", Label: "Monthly Pivot"},
			SRLevel{Price: pivots.S1, Type: "Support", Label: "Monthly S1"},
			SRLevel{Price: pivots.S2, Type: "Support", Label: "Monthly S2"},
			SRLevel{Price: pivots.S3, Type: "Support", Label: "Monthly S3"},
		)
	}

	return levels
}

// FindNearestSR finds the nearest support/resistance level to the current price.
func FindNearestSR(levels []SRLevel, currentPrice float64) ProximityResult {
	if len(levels) == 0 || currentPrice == 0 {
		return ProximityResult{}
	}

	var nearest SRLevel
	minDist := math.MaxFloat64

	for _, l := range levels {
		dist := math.Abs(currentPrice - l.Price)
		if dist < minDist {
			minDist = dist
			nearest = l
		}
	}

	pctDist := ((nearest.Price - currentPrice) / currentPrice) * 100

	return ProximityResult{
		Distance: pctDist,
		Type:     nearest.Type,
		Level:    nearest.Label,
		Price:    nearest.Price,
	}
}

// FindNearestMonthlySR finds the nearest monthly S/R level.
func FindNearestMonthlySR(monthlyCandles []Candle, currentPrice float64) SRResult {
	if len(monthlyCandles) < 2 || currentPrice == 0 {
		return SRResult{}
	}

	mh, ml, mc := ExtractMonthlyHLC(monthlyCandles)
	pivots := CalcMonthlyPivots(mh, ml, mc)

	levels := []struct {
		price float64
		label string
		typ   string
	}{
		{pivots.R3, "Monthly R3", "Resistance"},
		{pivots.R2, "Monthly R2", "Resistance"},
		{pivots.R1, "Monthly R1", "Resistance"},
		{pivots.P, "Monthly Pivot", "Support"},
		{pivots.S1, "Monthly S1", "Support"},
		{pivots.S2, "Monthly S2", "Support"},
		{pivots.S3, "Monthly S3", "Support"},
	}

	var nearest SRResult
	minDist := math.MaxFloat64

	for _, l := range levels {
		dist := math.Abs(currentPrice - l.price)
		if dist < minDist {
			minDist = dist
			pctDist := ((l.price - currentPrice) / currentPrice) * 100
			nearest = SRResult{
				Distance: pctDist,
				Type:     l.typ,
				Level:    l.label,
				Price:    l.price,
			}
		}
	}

	return nearest
}
