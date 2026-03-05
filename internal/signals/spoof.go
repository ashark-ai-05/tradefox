package signals

// LevelMap maps price → size for a book side.
type LevelMap map[float64]float64

// ComputeSpoof detects large orders that appeared then vanished.
func ComputeSpoof(prevBids, prevAsks LevelMap, currBids, currAsks []BookLevel) SpoofSignal {
	if prevBids == nil && prevAsks == nil {
		return SpoofSignal{}
	}

	currBidMap := toLevelMap(currBids)
	currAskMap := toLevelMap(currAsks)

	avgBidSize := avgSize(currBids)
	avgAskSize := avgSize(currAsks)

	var bss, ass float64

	for p, sz := range prevBids {
		if sz > avgBidSize*3 {
			if _, exists := currBidMap[p]; !exists {
				if avgBidSize > 0 {
					bss += sz / avgBidSize
				}
			}
		}
	}

	for p, sz := range prevAsks {
		if sz > avgAskSize*3 {
			if _, exists := currAskMap[p]; !exists {
				if avgAskSize > 0 {
					ass += sz / avgAskSize
				}
			}
		}
	}

	score := max64(bss, ass) / 10
	if score > 1 {
		score = 1
	}

	sig := SpoofSignal{Score: score}
	if score > 0.3 {
		sig.Active = true
		if bss > ass {
			sig.Side = "bid"
		} else {
			sig.Side = "ask"
		}
	}
	return sig
}

// BuildLevelMap creates a LevelMap from book levels.
func BuildLevelMap(levels []BookLevel) LevelMap {
	return toLevelMap(levels)
}

func toLevelMap(levels []BookLevel) LevelMap {
	m := make(LevelMap, len(levels))
	for _, l := range levels {
		m[l.Price] = l.Size
	}
	return m
}

func avgSize(levels []BookLevel) float64 {
	if len(levels) == 0 {
		return 0
	}
	sum := 0.0
	for _, l := range levels {
		sum += l.Size
	}
	return sum / float64(len(levels))
}

func max64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
