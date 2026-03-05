package models

import (
	"github.com/ashark-ai-05/tradefox/internal/core/enums"
)

// CalculateRealizedPnL calculates realized P&L from matched buy/sell orders.
// FIFO: matches oldest buys with oldest sells (index 0 to end).
// LIFO: matches newest buys with newest sells (end to index 0).
func CalculateRealizedPnL(buys, sells []Order, method enums.PositionCalcMethod) float64 {
	if len(buys) == 0 || len(sells) == 0 {
		return 0
	}

	// Copy remaining filled quantities so we don't mutate the originals.
	buyRemaining := make([]float64, len(buys))
	for i := range buys {
		buyRemaining[i] = buys[i].FilledQuantity
	}
	sellRemaining := make([]float64, len(sells))
	for i := range sells {
		sellRemaining[i] = sells[i].FilledQuantity
	}

	var realizedPnL float64

	switch method {
	case enums.PositionCalcFIFO:
		realizedPnL = matchFIFO(buys, sells, buyRemaining, sellRemaining)
	case enums.PositionCalcLIFO:
		realizedPnL = matchLIFO(buys, sells, buyRemaining, sellRemaining)
	}

	return realizedPnL
}

// CalculateOpenPnL calculates unrealized P&L for unmatched positions against
// the current mid price. After FIFO/LIFO matching consumes matched quantities,
// any remaining buy or sell positions contribute to open P&L.
func CalculateOpenPnL(buys, sells []Order, method enums.PositionCalcMethod, midPrice float64) float64 {
	if midPrice == 0 {
		return 0
	}

	// Copy remaining filled quantities so we don't mutate the originals.
	buyRemaining := make([]float64, len(buys))
	for i := range buys {
		buyRemaining[i] = buys[i].FilledQuantity
	}
	sellRemaining := make([]float64, len(sells))
	for i := range sells {
		sellRemaining[i] = sells[i].FilledQuantity
	}

	// First, consume matched quantities using the specified method.
	switch method {
	case enums.PositionCalcFIFO:
		matchFIFO(buys, sells, buyRemaining, sellRemaining)
	case enums.PositionCalcLIFO:
		matchLIFO(buys, sells, buyRemaining, sellRemaining)
	}

	// Calculate open P&L from unmatched positions.
	var openPnL float64

	for i, rem := range buyRemaining {
		if rem > 0 {
			openPnL += rem * (midPrice - buys[i].PricePlaced)
		}
	}

	for i, rem := range sellRemaining {
		if rem > 0 {
			openPnL += rem * (sells[i].PricePlaced - midPrice)
		}
	}

	return openPnL
}

// matchFIFO walks buys and sells from oldest (index 0) to newest, matching
// quantities and returning the realized P&L. It mutates the remaining slices.
func matchFIFO(buys, sells []Order, buyRemaining, sellRemaining []float64) float64 {
	var pnl float64
	bi, si := 0, 0

	for bi < len(buys) && si < len(sells) {
		if buyRemaining[bi] <= 0 {
			bi++
			continue
		}
		if sellRemaining[si] <= 0 {
			si++
			continue
		}

		matched := buyRemaining[bi]
		if sellRemaining[si] < matched {
			matched = sellRemaining[si]
		}

		pnl += matched * (sells[si].PricePlaced - buys[bi].PricePlaced)
		buyRemaining[bi] -= matched
		sellRemaining[si] -= matched
	}

	return pnl
}

// matchLIFO walks buys and sells from newest (end) to oldest (index 0),
// matching quantities and returning the realized P&L. It mutates the remaining
// slices.
func matchLIFO(buys, sells []Order, buyRemaining, sellRemaining []float64) float64 {
	var pnl float64
	bi := len(buys) - 1
	si := len(sells) - 1

	for bi >= 0 && si >= 0 {
		if buyRemaining[bi] <= 0 {
			bi--
			continue
		}
		if sellRemaining[si] <= 0 {
			si--
			continue
		}

		matched := buyRemaining[bi]
		if sellRemaining[si] < matched {
			matched = sellRemaining[si]
		}

		pnl += matched * (sells[si].PricePlaced - buys[bi].PricePlaced)
		buyRemaining[bi] -= matched
		sellRemaining[si] -= matched
	}

	return pnl
}
