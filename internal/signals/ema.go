package signals

const EMAAlpha = 0.3

// EMA computes exponential moving average. Returns curr if prev is 0 (first value).
func EMA(prev, curr float64) float64 {
	if prev == 0 {
		return curr
	}
	return prev + EMAAlpha*(curr-prev)
}

// EMAWithAlpha computes exponential moving average with a custom alpha.
func EMAWithAlpha(prev, curr, alpha float64) float64 {
	if prev == 0 {
		return curr
	}
	return prev + alpha*(curr-prev)
}
