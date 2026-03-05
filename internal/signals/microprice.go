package signals

// BookLevel is a simplified price level for signal computation.
type BookLevel struct {
	Price float64
	Size  float64
}

// ComputeMicroprice computes the microprice divergence signal.
// mid is the arithmetic mid price, micro is the size-weighted microprice.
func ComputeMicroprice(mid, micro, prevDivBps float64) MicropriceSignal {
	if micro == 0 {
		micro = mid
	}

	rawDivBps := 0.0
	if mid > 0 {
		rawDivBps = ((micro - mid) / mid) * 10000
	}

	divBps := EMA(prevDivBps, rawDivBps)

	dir := "neutral"
	if divBps > 0.5 {
		dir = "up"
	} else if divBps < -0.5 {
		dir = "down"
	}

	return MicropriceSignal{
		Value:  micro,
		Mid:    mid,
		DivBps: divBps,
		Dir:    dir,
	}
}
