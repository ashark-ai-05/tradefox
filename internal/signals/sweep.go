package signals

import "time"

// TradeRecord is a simplified trade for signal computation.
type TradeRecord struct {
	Price     float64
	Size      float64
	IsBuy     bool
	Timestamp time.Time
}

// ComputeSweepAt is like ComputeSweep but uses the provided time instead of time.Now().
func ComputeSweepAt(trades []TradeRecord, now time.Time) SweepSignal {
	if len(trades) < 5 {
		return SweepSignal{}
	}

	start := len(trades) - 30
	if start < 0 {
		start = 0
	}
	recent := trades[start:]

	for i := len(recent) - 1; i >= 2; i-- {
		anchor := recent[i]
		if now.Sub(anchor.Timestamp) > 5*time.Second {
			break
		}

		prices := make(map[float64]struct{})
		var sz float64
		dir := anchor.IsBuy

		for j := i; j >= 0; j-- {
			t := recent[j]
			if anchor.Timestamp.Sub(t.Timestamp) > 500*time.Millisecond {
				break
			}
			if t.IsBuy != dir {
				break
			}
			prices[t.Price] = struct{}{}
			sz += t.Size
		}

		if len(prices) >= 3 {
			d := "sell"
			if dir {
				d = "buy"
			}
			return SweepSignal{
				Active: true,
				Dir:    d,
				Levels: len(prices),
				Size:   sz,
			}
		}
	}

	return SweepSignal{}
}

// ComputeSweep detects aggressive orders. Delegates to ComputeSweepAt with time.Now().
func ComputeSweep(trades []TradeRecord) SweepSignal {
	return ComputeSweepAt(trades, time.Now())
}
