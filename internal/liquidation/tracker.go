package liquidation

import (
	"sync"
	"time"
)

// PositionEstimate represents an estimated position opened at a given price level.
type PositionEstimate struct {
	EntryPrice float64 `json:"entryPrice"`
	Volume     float64 `json:"volume"`
	Timestamp  int64   `json:"timestamp"`
	Side       string  `json:"side"` // "long" or "short"
}

// Tracker accumulates position estimates from OI changes over time.
// When open interest increases at a price, it records estimated new positions.
type Tracker struct {
	mu        sync.RWMutex
	positions map[string][]PositionEstimate // symbol → position estimates
}

// NewTracker creates a new position accumulation tracker.
func NewTracker() *Tracker {
	return &Tracker{
		positions: make(map[string][]PositionEstimate),
	}
}

// ProcessOIChange records estimated new positions when OI increases at a price.
// A positive oiDelta indicates new positions opened; negative means positions closed.
// For positive deltas, we split the volume 50/50 between longs and shorts since
// every futures contract has both a long and short side.
func (t *Tracker) ProcessOIChange(symbol string, price float64, oiDelta float64, timestamp int64) {
	if oiDelta <= 0 {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	halfVol := oiDelta / 2.0

	t.positions[symbol] = append(t.positions[symbol],
		PositionEstimate{
			EntryPrice: price,
			Volume:     halfVol,
			Timestamp:  timestamp,
			Side:       "long",
		},
		PositionEstimate{
			EntryPrice: price,
			Volume:     halfVol,
			Timestamp:  timestamp,
			Side:       "short",
		},
	)
}

// DecayOldPositions removes position estimates older than maxAge.
func (t *Tracker) DecayOldPositions(maxAge time.Duration) {
	cutoff := time.Now().UnixMilli() - maxAge.Milliseconds()

	t.mu.Lock()
	defer t.mu.Unlock()

	for symbol, positions := range t.positions {
		kept := positions[:0]
		for _, p := range positions {
			if p.Timestamp >= cutoff {
				kept = append(kept, p)
			}
		}
		if len(kept) == 0 {
			delete(t.positions, symbol)
		} else {
			t.positions[symbol] = kept
		}
	}
}

// GetPositionMap returns a copy of all position estimates for a symbol.
func (t *Tracker) GetPositionMap(symbol string) []PositionEstimate {
	t.mu.RLock()
	defer t.mu.RUnlock()

	src := t.positions[symbol]
	if len(src) == 0 {
		return nil
	}

	out := make([]PositionEstimate, len(src))
	copy(out, src)
	return out
}

// PositionCount returns the number of tracked position estimates for a symbol.
func (t *Tracker) PositionCount(symbol string) int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.positions[symbol])
}
