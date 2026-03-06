package scanner

import "math"

// CalcRSI computes the Wilder RSI for the given candles over the specified period.
func CalcRSI(candles []Candle, period int) float64 {
	if len(candles) < period+1 {
		return 50 // not enough data, return neutral
	}

	var avgGain, avgLoss float64

	// Initial average over the first `period` changes
	for i := 1; i <= period; i++ {
		change := candles[i].Close - candles[i-1].Close
		if change > 0 {
			avgGain += change
		} else {
			avgLoss -= change
		}
	}
	avgGain /= float64(period)
	avgLoss /= float64(period)

	// Smooth using Wilder's method for remaining candles
	for i := period + 1; i < len(candles); i++ {
		change := candles[i].Close - candles[i-1].Close
		if change > 0 {
			avgGain = (avgGain*float64(period-1) + change) / float64(period)
			avgLoss = (avgLoss * float64(period-1)) / float64(period)
		} else {
			avgGain = (avgGain * float64(period-1)) / float64(period)
			avgLoss = (avgLoss*float64(period-1) - change) / float64(period)
		}
	}

	if avgLoss == 0 {
		return 100
	}

	rs := avgGain / avgLoss
	return 100 - (100 / (1 + rs))
}

// CalcRSIHistory returns the last historyLen RSI values for sparkline rendering.
func CalcRSIHistory(candles []Candle, period, historyLen int) []float64 {
	if len(candles) < period+1 {
		return nil
	}

	// Compute RSI for each position from period+1 onward
	var avgGain, avgLoss float64
	for i := 1; i <= period; i++ {
		change := candles[i].Close - candles[i-1].Close
		if change > 0 {
			avgGain += change
		} else {
			avgLoss -= change
		}
	}
	avgGain /= float64(period)
	avgLoss /= float64(period)

	all := make([]float64, 0, len(candles)-period)

	// First RSI value
	if avgLoss == 0 {
		all = append(all, 100)
	} else {
		rs := avgGain / avgLoss
		all = append(all, 100-(100/(1+rs)))
	}

	for i := period + 1; i < len(candles); i++ {
		change := candles[i].Close - candles[i-1].Close
		if change > 0 {
			avgGain = (avgGain*float64(period-1) + change) / float64(period)
			avgLoss = (avgLoss * float64(period-1)) / float64(period)
		} else {
			avgGain = (avgGain * float64(period-1)) / float64(period)
			avgLoss = (avgLoss*float64(period-1) - change) / float64(period)
		}

		if avgLoss == 0 {
			all = append(all, 100)
		} else {
			rs := avgGain / avgLoss
			all = append(all, 100-(100/(1+rs)))
		}
	}

	// Return last historyLen values
	if len(all) <= historyLen {
		return all
	}
	return all[len(all)-historyLen:]
}

// ClassifyRSI categorizes an RSI value into a state string.
func ClassifyRSI(value float64) string {
	value = math.Round(value*100) / 100
	switch {
	case value < 20:
		return "StrongOversold"
	case value < 30:
		return "Oversold"
	case value < 40:
		return "Weak"
	case value < 60:
		return "Neutral"
	case value < 70:
		return "Strong"
	case value < 80:
		return "Overbought"
	default:
		return "StrongOverbought"
	}
}

// ComputeRSIForTimeframes computes RSI values and history for all scanner timeframes.
func ComputeRSIForTimeframes(klines map[string][]Candle) (values map[string]float64, history map[string][]float64) {
	values = make(map[string]float64)
	history = make(map[string][]float64)

	for _, tf := range RSITimeframes {
		candles, ok := klines[tf]
		if !ok || len(candles) < 15 {
			values[tf] = 50
			continue
		}
		values[tf] = CalcRSI(candles, 14)
		history[tf] = CalcRSIHistory(candles, 14, 20)
	}
	return
}

// AggregateRSIState determines the overall RSI state from the 1h RSI value.
func AggregateRSIState(values map[string]float64) string {
	// Use 1h as the primary RSI for classification
	if v, ok := values["1h"]; ok {
		return ClassifyRSI(v)
	}
	return "Neutral"
}
