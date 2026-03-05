package backtest

import (
	"math"
)

// PositionConfig holds sizing and stop parameters.
type PositionConfig struct {
	KellyFraction float64 // fraction of Kelly (default: 0.25)
	MaxRiskPct    float64 // max risk per trade as fraction of equity (default: 0.005)

	StopATRMult   float64 // stop distance in ATR multiples (default: 1.5)
	TargetATRMult float64 // target distance in ATR multiples (default: 3.0)

	// 3-phase trailing stop
	Phase1Trigger float64 // fraction of target (default: 0.5)
	Phase1Trail   float64 // trail in ATR (default: 0.3)
	Phase2Trigger float64 // default: 0.8
	Phase2Trail   float64 // default: 0.15
	Phase3Trigger float64 // default: 1.0
	Phase3Trail   float64 // default: 0.08

	MaxHoldingMs int64 // max holding period in ms (default: 4 hours)
}

func DefaultPositionConfig() PositionConfig {
	return PositionConfig{
		KellyFraction: 0.25,
		MaxRiskPct:    0.005,
		StopATRMult:   1.5,
		TargetATRMult: 3.0,
		Phase1Trigger: 0.5,
		Phase1Trail:   0.3,
		Phase2Trigger: 0.8,
		Phase2Trail:   0.15,
		Phase3Trigger: 1.0,
		Phase3Trail:   0.08,
		MaxHoldingMs:  4 * 60 * 60 * 1000, // 4 hours
	}
}

// Position represents an open trading position.
type Position struct {
	Symbol      string
	Direction   int     // +1 long, -1 short
	EntryPrice  float64
	Size        float64
	EntryTime   int64
	StopPrice   float64
	TargetPrice float64
	HighWater   float64 // highest favorable price
	Phase       int     // trailing stop phase (0=initial, 1, 2, 3)
	ATR         float64 // ATR at entry
}

// ClosedTrade is a completed trade with P&L.
type ClosedTrade struct {
	Symbol     string  `json:"symbol"`
	Direction  int     `json:"direction"`
	EntryPrice float64 `json:"entryPrice"`
	ExitPrice  float64 `json:"exitPrice"`
	Size       float64 `json:"size"`
	EntryTime  int64   `json:"entryTime"`
	ExitTime   int64   `json:"exitTime"`
	PnLPct     float64 `json:"pnlPct"`
	PnLAbs     float64 `json:"pnlAbs"`
	ExitReason string  `json:"exitReason"` // "stop", "target", "trailing", "timeout"
	Fees       float64 `json:"fees"`
}

// PositionManager tracks open positions and handles stop/target logic.
type PositionManager struct {
	config    PositionConfig
	positions map[string]*Position
	equity    float64
}

func NewPositionManager(cfg PositionConfig, initialEquity float64) *PositionManager {
	return &PositionManager{
		config:    cfg,
		positions: make(map[string]*Position),
		equity:    initialEquity,
	}
}

// HasPosition returns true if there's an open position for the symbol.
func (pm *PositionManager) HasPosition(symbol string) bool {
	_, ok := pm.positions[symbol]
	return ok
}

// ComputeSize calculates position size using quarter-Kelly with ATR-based risk.
func (pm *PositionManager) ComputeSize(atr, entryPrice float64) float64 {
	if atr <= 0 || entryPrice <= 0 {
		return 0
	}
	// Risk per unit = ATR * stop multiplier
	riskPerUnit := atr * pm.config.StopATRMult
	// Max position size from risk limit
	maxRisk := pm.equity * pm.config.MaxRiskPct
	size := maxRisk / riskPerUnit

	// Apply Kelly fraction
	size *= pm.config.KellyFraction

	return size
}

// OpenPosition opens a new position.
func (pm *PositionManager) OpenPosition(symbol string, direction int, entryPrice, size, atr float64, timestamp int64) {
	stopDist := atr * pm.config.StopATRMult
	targetDist := atr * pm.config.TargetATRMult

	var stopPrice, targetPrice float64
	if direction == 1 { // long
		stopPrice = entryPrice - stopDist
		targetPrice = entryPrice + targetDist
	} else { // short
		stopPrice = entryPrice + stopDist
		targetPrice = entryPrice - targetDist
	}

	pm.positions[symbol] = &Position{
		Symbol:      symbol,
		Direction:   direction,
		EntryPrice:  entryPrice,
		Size:        size,
		EntryTime:   timestamp,
		StopPrice:   stopPrice,
		TargetPrice: targetPrice,
		HighWater:   entryPrice,
		Phase:       0,
		ATR:         atr,
	}
}

// Update checks stops, targets, trailing, and timeout against current price.
// Returns a ClosedTrade if the position was closed, nil otherwise.
func (pm *PositionManager) Update(symbol string, currentPrice float64, timestamp int64, fees float64) *ClosedTrade {
	pos, ok := pm.positions[symbol]
	if !ok {
		return nil
	}

	// Update high water mark
	if pos.Direction == 1 && currentPrice > pos.HighWater {
		pos.HighWater = currentPrice
	}
	if pos.Direction == -1 && currentPrice < pos.HighWater {
		pos.HighWater = currentPrice
	}

	// Check trailing stop phases (must happen before stop check)
	pm.updateTrailingStop(pos)

	// Check exit conditions
	var exitReason string

	// Timeout
	if timestamp-pos.EntryTime > pm.config.MaxHoldingMs {
		exitReason = "timeout"
	}

	// Stop loss
	if exitReason == "" {
		if pos.Direction == 1 && currentPrice <= pos.StopPrice {
			if pos.Phase > 0 {
				exitReason = "trailing"
			} else {
				exitReason = "stop"
			}
		}
		if pos.Direction == -1 && currentPrice >= pos.StopPrice {
			if pos.Phase > 0 {
				exitReason = "trailing"
			} else {
				exitReason = "stop"
			}
		}
	}

	// Target (only if not already in phase 3 trailing and not already exiting)
	if exitReason == "" && pos.Phase < 3 {
		if pos.Direction == 1 && currentPrice >= pos.TargetPrice {
			exitReason = "target"
		}
		if pos.Direction == -1 && currentPrice <= pos.TargetPrice {
			exitReason = "target"
		}
	}

	if exitReason == "" {
		return nil
	}

	return pm.closePosition(symbol, currentPrice, timestamp, exitReason, fees)
}

func (pm *PositionManager) updateTrailingStop(pos *Position) {
	progress := pm.positionProgress(pos)

	cfg := pm.config
	newPhase := pos.Phase

	if progress >= cfg.Phase3Trigger && pos.Phase < 3 {
		newPhase = 3
	} else if progress >= cfg.Phase2Trigger && pos.Phase < 2 {
		newPhase = 2
	} else if progress >= cfg.Phase1Trigger && pos.Phase < 1 {
		newPhase = 1
	}

	if newPhase > pos.Phase {
		pos.Phase = newPhase
	}

	// Update stop based on phase
	if pos.Phase > 0 {
		var trailDist float64
		switch pos.Phase {
		case 1:
			trailDist = cfg.Phase1Trail * pos.ATR
		case 2:
			trailDist = cfg.Phase2Trail * pos.ATR
		case 3:
			trailDist = cfg.Phase3Trail * pos.ATR
		}

		var newStop float64
		if pos.Direction == 1 {
			newStop = pos.HighWater - trailDist
		} else {
			newStop = pos.HighWater + trailDist
		}

		// Only move stop in favorable direction
		if pos.Direction == 1 && newStop > pos.StopPrice {
			pos.StopPrice = newStop
		}
		if pos.Direction == -1 && newStop < pos.StopPrice {
			pos.StopPrice = newStop
		}
	}
}

func (pm *PositionManager) positionProgress(pos *Position) float64 {
	totalDist := math.Abs(pos.TargetPrice - pos.EntryPrice)
	if totalDist == 0 {
		return 0
	}
	var currentDist float64
	if pos.Direction == 1 {
		currentDist = pos.HighWater - pos.EntryPrice
	} else {
		currentDist = pos.EntryPrice - pos.HighWater
	}
	return currentDist / totalDist
}

func (pm *PositionManager) closePosition(symbol string, exitPrice float64, timestamp int64, reason string, fees float64) *ClosedTrade {
	pos := pm.positions[symbol]
	delete(pm.positions, symbol)

	var pnlPct float64
	if pos.Direction == 1 {
		pnlPct = (exitPrice - pos.EntryPrice) / pos.EntryPrice
	} else {
		pnlPct = (pos.EntryPrice - exitPrice) / pos.EntryPrice
	}
	pnlAbs := pnlPct * pos.Size * pos.EntryPrice

	// Update equity
	pm.equity += pnlAbs - fees

	return &ClosedTrade{
		Symbol:     symbol,
		Direction:  pos.Direction,
		EntryPrice: pos.EntryPrice,
		ExitPrice:  exitPrice,
		Size:       pos.Size,
		EntryTime:  pos.EntryTime,
		ExitTime:   timestamp,
		PnLPct:     pnlPct,
		PnLAbs:     pnlAbs,
		ExitReason: reason,
		Fees:       fees,
	}
}

// CloseAll closes all open positions at the given price. Used at end of backtest.
func (pm *PositionManager) CloseAll(currentPrices map[string]float64, timestamp int64) []ClosedTrade {
	var trades []ClosedTrade
	for symbol := range pm.positions {
		price := currentPrices[symbol]
		if price <= 0 {
			continue
		}
		ct := pm.closePosition(symbol, price, timestamp, "end_of_data", 0)
		if ct != nil {
			trades = append(trades, *ct)
		}
	}
	return trades
}

// Equity returns the current equity.
func (pm *PositionManager) Equity() float64 {
	return pm.equity
}
