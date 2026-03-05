package backtest

// ATRCalculator computes Average True Range from OHLCV candles.
type ATRCalculator struct {
	period    int
	values    []float64
	prevClose float64
	current   float64
}

func NewATRCalculator(period int) *ATRCalculator {
	return &ATRCalculator{period: period}
}

// Update feeds a new candle and returns the current ATR.
func (a *ATRCalculator) Update(high, low, close float64) float64 {
	var tr float64
	if a.prevClose == 0 {
		tr = high - low
	} else {
		hl := high - low
		hc := absFloat(high - a.prevClose)
		lc := absFloat(low - a.prevClose)
		tr = max3(hl, hc, lc)
	}
	a.prevClose = close

	a.values = append(a.values, tr)
	if len(a.values) > a.period {
		a.values = a.values[len(a.values)-a.period:]
	}

	// Simple average
	var sum float64
	for _, v := range a.values {
		sum += v
	}
	a.current = sum / float64(len(a.values))
	return a.current
}

// Current returns the latest ATR value.
func (a *ATRCalculator) Current() float64 {
	return a.current
}

func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func max3(a, b, c float64) float64 {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	return m
}
