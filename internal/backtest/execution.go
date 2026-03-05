package backtest

// ExecutionConfig holds execution simulation parameters.
type ExecutionConfig struct {
	MakerFeeBps float64 // default: 2.0 (0.02%)
	TakerFeeBps float64 // default: 5.0 (0.05%)
	SpreadMult  float64 // spread multiplier for slippage (default: 1.0)
}

func DefaultExecutionConfig() ExecutionConfig {
	return ExecutionConfig{
		MakerFeeBps: 2.0,
		TakerFeeBps: 5.0,
		SpreadMult:  1.0,
	}
}

// PendingOrder is an order waiting to be filled at the next book update.
type PendingOrder struct {
	Symbol    string
	Direction int // +1 buy, -1 sell
	Size      float64
	PlacedAt  int64
}

// Fill represents a completed order fill.
type Fill struct {
	Price     float64
	Size      float64
	Fees      float64
	Slippage  float64
	Timestamp int64
}

// ExecutionSimulator models realistic order fills.
type ExecutionSimulator struct {
	config  ExecutionConfig
	pending *PendingOrder
}

func NewExecutionSimulator(cfg ExecutionConfig) *ExecutionSimulator {
	return &ExecutionSimulator{config: cfg}
}

// PlaceOrder queues an order for fill at the next book update.
func (e *ExecutionSimulator) PlaceOrder(symbol string, direction int, size float64, timestamp int64) {
	e.pending = &PendingOrder{
		Symbol:    symbol,
		Direction: direction,
		Size:      size,
		PlacedAt:  timestamp,
	}
}

// HasPending returns true if there's a pending order.
func (e *ExecutionSimulator) HasPending() bool {
	return e.pending != nil
}

// TryFill attempts to fill the pending order at the given book state.
// Returns nil if no pending order or if timestamp is the same as placement.
// Fill price = mid + (direction * spread/2) + slippage
// Slippage = size * lambda * spreadMult
func (e *ExecutionSimulator) TryFill(midPrice, spread, lambda float64, timestamp int64) *Fill {
	if e.pending == nil {
		return nil
	}

	// Must be at a DIFFERENT timestamp (next book update)
	if timestamp <= e.pending.PlacedAt {
		return nil
	}

	order := e.pending
	e.pending = nil

	// Crossing the spread
	halfSpread := spread / 2.0
	crossCost := float64(order.Direction) * halfSpread

	// Slippage based on lambda (price impact)
	slippage := order.Size * lambda * e.config.SpreadMult
	if slippage < 0 {
		slippage = 0
	}

	fillPrice := midPrice + crossCost + float64(order.Direction)*slippage

	// Fees (taker since we're crossing the spread)
	notional := fillPrice * order.Size
	fees := notional * e.config.TakerFeeBps / 10000.0

	return &Fill{
		Price:     fillPrice,
		Size:      order.Size,
		Fees:      fees,
		Slippage:  slippage,
		Timestamp: timestamp,
	}
}

// Cancel cancels any pending order.
func (e *ExecutionSimulator) Cancel() {
	e.pending = nil
}
