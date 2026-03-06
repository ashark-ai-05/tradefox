package scanner

import (
	"math"
	"sort"
)

// Leverage tiers with their estimated market share weights.
var leverageTiers = []struct {
	Leverage float64
	Weight   float64
}{
	{3, 0.10},
	{5, 0.15},
	{10, 0.30},
	{20, 0.25},
	{25, 0.10},
	{50, 0.07},
	{100, 0.03},
}

// EstimateLiqClusters estimates liquidation clusters from recent candle data.
func EstimateLiqClusters(currentPrice float64, recentCandles []Candle) LiqEstimate {
	if len(recentCandles) == 0 || currentPrice == 0 {
		return LiqEstimate{}
	}

	type liqLevel struct {
		price     float64
		estVolume float64
		isAbove   bool
	}

	var levels []liqLevel

	for _, candle := range recentCandles {
		if candle.Volume == 0 {
			continue
		}

		// Use candle OHLC as potential entry prices
		entries := []float64{candle.Open, candle.High, candle.Low, candle.Close}

		for _, entry := range entries {
			if entry == 0 {
				continue
			}

			for _, tier := range leverageTiers {
				// Long liquidation: entry * (1 - 1/leverage)
				longLiq := entry * (1 - 1/tier.Leverage)
				// Short liquidation: entry * (1 + 1/leverage)
				shortLiq := entry * (1 + 1/tier.Leverage)

				volContrib := candle.Volume * tier.Weight * 0.25 // split across 4 entry prices

				if longLiq < currentPrice {
					levels = append(levels, liqLevel{
						price:     longLiq,
						estVolume: volContrib,
						isAbove:   false,
					})
				}

				if shortLiq > currentPrice {
					levels = append(levels, liqLevel{
						price:     shortLiq,
						estVolume: volContrib,
						isAbove:   true,
					})
				}
			}
		}
	}

	if len(levels) == 0 {
		return LiqEstimate{}
	}

	// Aggregate nearby levels (within 0.5% of each other)
	sort.Slice(levels, func(i, j int) bool { return levels[i].price < levels[j].price })

	type cluster struct {
		price     float64
		volume    float64
		isAbove   bool
	}

	var clusters []cluster
	current := cluster{price: levels[0].price, volume: levels[0].estVolume, isAbove: levels[0].isAbove}

	for i := 1; i < len(levels); i++ {
		if math.Abs(levels[i].price-current.price)/current.price < 0.005 && levels[i].isAbove == current.isAbove {
			current.volume += levels[i].estVolume
			current.price = (current.price + levels[i].price) / 2 // weighted avg
		} else {
			clusters = append(clusters, current)
			current = cluster{price: levels[i].price, volume: levels[i].estVolume, isAbove: levels[i].isAbove}
		}
	}
	clusters = append(clusters, current)

	// Find nearest above and below
	var nearestAbove, nearestBelow LiqCluster
	var totalAbove, totalBelow float64
	minAboveDist := math.MaxFloat64
	minBelowDist := math.MaxFloat64

	for _, c := range clusters {
		dist := math.Abs(c.price-currentPrice) / currentPrice * 100
		if c.isAbove {
			totalAbove += c.volume
			if dist < minAboveDist {
				minAboveDist = dist
				nearestAbove = LiqCluster{Price: c.price, EstVolume: c.volume, Distance: dist}
			}
		} else {
			totalBelow += c.volume
			if dist < minBelowDist {
				minBelowDist = dist
				nearestBelow = LiqCluster{Price: c.price, EstVolume: c.volume, Distance: dist}
			}
		}
	}

	asymmetry := 1.0
	if totalBelow > 0 {
		asymmetry = totalAbove / totalBelow
	}

	return LiqEstimate{
		AbovePrice:   totalAbove,
		BelowPrice:   totalBelow,
		NearestAbove: nearestAbove,
		NearestBelow: nearestBelow,
		Asymmetry:    asymmetry,
	}
}
