package signals

import "math"

// OFIState holds the previous order book state needed for OFI computation.
type OFIState struct {
	BestBidPrice float64
	BestBidSize  float64
	BestAskPrice float64
	BestAskSize  float64
}

// ComputeOFI computes Order Flow Imbalance from current and previous best levels.
func ComputeOFI(prev OFIState, currBidPrice, currBidSize, currAskPrice, currAskSize, prevSmoothed float64) (OFISignal, OFIState) {
	rawOFI := 0.0

	if prev.BestBidPrice > 0 && prev.BestAskPrice > 0 {
		// Bid delta
		var bd float64
		if currBidPrice > prev.BestBidPrice {
			bd = currBidSize
		} else if currBidPrice == prev.BestBidPrice {
			bd = currBidSize - prev.BestBidSize
		} else {
			bd = -prev.BestBidSize
		}

		// Ask delta
		var ad float64
		if currAskPrice < prev.BestAskPrice {
			ad = currAskSize
		} else if currAskPrice == prev.BestAskPrice {
			ad = currAskSize - prev.BestAskSize
		} else {
			ad = -prev.BestAskSize
		}

		raw := bd - ad
		norm := math.Max(currBidSize+currAskSize, 1)
		rawOFI = math.Max(-1, math.Min(1, raw/norm))
	}

	val := EMA(prevSmoothed, rawOFI)

	dir := "neutral"
	if val > 0.15 {
		dir = "buy"
	} else if val < -0.15 {
		dir = "sell"
	}

	newState := OFIState{
		BestBidPrice: currBidPrice,
		BestBidSize:  currBidSize,
		BestAskPrice: currAskPrice,
		BestAskSize:  currAskSize,
	}

	return OFISignal{Value: val, Dir: dir}, newState
}
