package liquidation

import "math"

// LiquidationLevel represents a computed liquidation price for a position at a specific leverage.
type LiquidationLevel struct {
	Price      float64 `json:"price"`
	Volume     float64 `json:"volume"`
	Side       string  `json:"side"`       // "long" or "short"
	Leverage   int     `json:"leverage"`
	EntryPrice float64 `json:"entryPrice"`
}

// HeatmapBand represents an aggregated price bucket in the heatmap.
type HeatmapBand struct {
	PriceMin       float64 `json:"priceMin"`
	PriceMax       float64 `json:"priceMax"`
	LongLiqVolume  float64 `json:"longLiqVolume"`
	ShortLiqVolume float64 `json:"shortLiqVolume"`
	Intensity      float64 `json:"intensity"` // 0.0-1.0 normalized
	Side           string  `json:"side"`      // dominant side: "long", "short", or "neutral"
}

// leverageTier defines a leverage level and its estimated weight among traders.
type leverageTier struct {
	Leverage int
	Weight   float64
}

// Common leverage distribution weights based on exchange data analysis.
var defaultLeverageTiers = []leverageTier{
	{3, 0.10},
	{5, 0.15},
	{10, 0.30},
	{20, 0.25},
	{25, 0.10},
	{50, 0.07},
	{100, 0.03},
}

// CalcLiquidationPrice computes the estimated liquidation price for a position.
// Long: entry * (1 - 1/leverage)
// Short: entry * (1 + 1/leverage)
func CalcLiquidationPrice(entry float64, leverage float64, side string) float64 {
	if leverage <= 0 {
		return 0
	}
	switch side {
	case "long":
		return entry * (1.0 - 1.0/leverage)
	case "short":
		return entry * (1.0 + 1.0/leverage)
	default:
		return 0
	}
}

// FanOutLiquidations produces liquidation levels for each leverage tier,
// distributing the position's volume according to the tier weights.
func FanOutLiquidations(position PositionEstimate) []LiquidationLevel {
	levels := make([]LiquidationLevel, 0, len(defaultLeverageTiers))
	for _, tier := range defaultLeverageTiers {
		liqPrice := CalcLiquidationPrice(position.EntryPrice, float64(tier.Leverage), position.Side)
		if liqPrice <= 0 {
			continue
		}
		levels = append(levels, LiquidationLevel{
			Price:      liqPrice,
			Volume:     position.Volume * tier.Weight,
			Side:       position.Side,
			Leverage:   tier.Leverage,
			EntryPrice: position.EntryPrice,
		})
	}
	return levels
}

// AggregateLiquidations bins liquidation levels into N price buckets centered
// around the current price within ±priceRange% of the current price.
func AggregateLiquidations(levels []LiquidationLevel, currentPrice float64, numBins int, priceRange float64) []HeatmapBand {
	if numBins <= 0 || currentPrice <= 0 || priceRange <= 0 {
		return nil
	}

	rangeFraction := priceRange / 100.0
	low := currentPrice * (1.0 - rangeFraction)
	high := currentPrice * (1.0 + rangeFraction)
	binWidth := (high - low) / float64(numBins)

	bands := make([]HeatmapBand, numBins)
	for i := range bands {
		bands[i].PriceMin = low + float64(i)*binWidth
		bands[i].PriceMax = low + float64(i+1)*binWidth
	}

	for _, lev := range levels {
		if lev.Price < low || lev.Price >= high {
			continue
		}
		idx := int((lev.Price - low) / binWidth)
		if idx >= numBins {
			idx = numBins - 1
		}
		switch lev.Side {
		case "long":
			bands[idx].LongLiqVolume += lev.Volume
		case "short":
			bands[idx].ShortLiqVolume += lev.Volume
		}
	}

	// Normalize intensity and determine dominant side
	var maxVol float64
	for _, b := range bands {
		total := b.LongLiqVolume + b.ShortLiqVolume
		if total > maxVol {
			maxVol = total
		}
	}

	for i := range bands {
		total := bands[i].LongLiqVolume + bands[i].ShortLiqVolume
		if maxVol > 0 {
			bands[i].Intensity = math.Min(total/maxVol, 1.0)
		}
		switch {
		case bands[i].LongLiqVolume > bands[i].ShortLiqVolume:
			bands[i].Side = "long"
		case bands[i].ShortLiqVolume > bands[i].LongLiqVolume:
			bands[i].Side = "short"
		default:
			bands[i].Side = "neutral"
		}
	}

	return bands
}
