package execution

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/core/models"
)

// RiskLimits defines the risk parameters for pre-trade checks.
type RiskLimits struct {
	MaxPositionSize float64 `json:"maxPositionSize"` // max quantity per symbol
	MaxNotional     float64 `json:"maxNotional"`     // max notional per order (qty * price)
	DailyLossLimit  float64 `json:"dailyLossLimit"`  // max daily loss before kill switch
}

// RiskStatus is the current state of the risk manager.
type RiskStatus struct {
	DailyPnL       float64   `json:"dailyPnl"`
	DailyLossLimit float64   `json:"dailyLossLimit"`
	KillSwitch     bool      `json:"killSwitch"`
	LastUpdated    time.Time `json:"lastUpdated"`
}

// RiskManager enforces pre-trade risk checks and tracks daily P&L.
type RiskManager struct {
	mu         sync.RWMutex
	limits     RiskLimits
	dailyPnL   float64
	killSwitch bool
	resetDate  time.Time
	logger     *slog.Logger
}

// NewRiskManager creates a RiskManager with the given limits.
func NewRiskManager(limits RiskLimits, logger *slog.Logger) *RiskManager {
	return &RiskManager{
		limits:    limits,
		resetDate: today(),
		logger:    logger,
	}
}

// CheckPreTrade validates an order against risk limits.
// Returns nil if the order passes, or an error describing the violation.
func (rm *RiskManager) CheckPreTrade(order *models.Order) error {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if rm.killSwitch {
		return fmt.Errorf("kill switch is active, all trading halted")
	}

	// Reset daily P&L if it's a new day
	rm.maybeResetDay()

	if rm.limits.MaxPositionSize > 0 && order.Quantity > rm.limits.MaxPositionSize {
		return fmt.Errorf("order quantity %.4f exceeds max position size %.4f", order.Quantity, rm.limits.MaxPositionSize)
	}

	notional := order.Quantity * order.PricePlaced
	if order.PricePlaced == 0 {
		// For market orders, we can't check notional without price
		return nil
	}
	if rm.limits.MaxNotional > 0 && notional > rm.limits.MaxNotional {
		return fmt.Errorf("order notional %.2f exceeds max notional %.2f", notional, rm.limits.MaxNotional)
	}

	return nil
}

// UpdateOnFill updates the daily P&L when an order is filled.
// realizedPnL is the P&L from this fill (positive = profit, negative = loss).
func (rm *RiskManager) UpdateOnFill(realizedPnL float64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.maybeResetDay()
	rm.dailyPnL += realizedPnL

	if rm.limits.DailyLossLimit > 0 && rm.dailyPnL <= -rm.limits.DailyLossLimit {
		rm.killSwitch = true
		rm.logger.Error("KILL SWITCH ACTIVATED: daily loss limit breached",
			slog.Float64("daily_pnl", rm.dailyPnL),
			slog.Float64("limit", rm.limits.DailyLossLimit),
		)
	}
}

// ActivateKillSwitch manually activates the kill switch.
func (rm *RiskManager) ActivateKillSwitch() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.killSwitch = true
	rm.logger.Warn("kill switch manually activated")
}

// DeactivateKillSwitch manually deactivates the kill switch.
func (rm *RiskManager) DeactivateKillSwitch() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.killSwitch = false
	rm.logger.Warn("kill switch manually deactivated")
}

// Status returns the current risk status.
func (rm *RiskManager) Status() RiskStatus {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return RiskStatus{
		DailyPnL:       rm.dailyPnL,
		DailyLossLimit: rm.limits.DailyLossLimit,
		KillSwitch:     rm.killSwitch,
		LastUpdated:    time.Now(),
	}
}

// IsKillSwitchActive returns whether the kill switch is currently on.
func (rm *RiskManager) IsKillSwitchActive() bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.killSwitch
}

func (rm *RiskManager) maybeResetDay() {
	t := today()
	if t.After(rm.resetDate) {
		rm.dailyPnL = 0
		rm.killSwitch = false
		rm.resetDate = t
	}
}

func today() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}
