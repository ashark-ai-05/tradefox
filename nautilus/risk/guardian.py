from dataclasses import dataclass, field


@dataclass
class RiskGuardian:
    """Enforces risk limits before order submission."""

    max_position_size_pct: float = 2.0
    daily_loss_limit_pct: float = 3.0
    weekly_loss_limit_pct: float = 5.0
    max_drawdown_pct: float = 10.0
    kill_switch_enabled: bool = True

    # Runtime state
    total_equity: float = 10000.0
    daily_pnl: float = 0.0
    weekly_pnl: float = 0.0
    peak_equity: float = 10000.0
    kill_switch_active: bool = False

    def check_order(self, order_size_usd: float) -> tuple[bool, str]:
        """Check if an order passes risk limits.

        Returns (allowed, reason).
        """
        if self.kill_switch_active:
            return False, "kill switch active"

        # Position size check
        if self.total_equity > 0:
            size_pct = (order_size_usd / self.total_equity) * 100
            if size_pct > self.max_position_size_pct:
                return False, f"position size {size_pct:.1f}% exceeds limit {self.max_position_size_pct}%"

        # Daily loss check
        if self.total_equity > 0:
            daily_loss_pct = abs(min(self.daily_pnl, 0)) / self.total_equity * 100
            if daily_loss_pct >= self.daily_loss_limit_pct:
                if self.kill_switch_enabled:
                    self.kill_switch_active = True
                return False, f"daily loss limit reached ({daily_loss_pct:.1f}%)"

        # Weekly loss check
        if self.total_equity > 0:
            weekly_loss_pct = abs(min(self.weekly_pnl, 0)) / self.total_equity * 100
            if weekly_loss_pct >= self.weekly_loss_limit_pct:
                if self.kill_switch_enabled:
                    self.kill_switch_active = True
                return False, f"weekly loss limit reached ({weekly_loss_pct:.1f}%)"

        # Max drawdown check
        if self.peak_equity > 0:
            drawdown_pct = (self.peak_equity - self.total_equity) / self.peak_equity * 100
            if drawdown_pct >= self.max_drawdown_pct:
                if self.kill_switch_enabled:
                    self.kill_switch_active = True
                return False, f"max drawdown reached ({drawdown_pct:.1f}%)"

        return True, "ok"

    def update_equity(self, equity: float):
        """Update equity and peak tracking."""
        self.total_equity = equity
        if equity > self.peak_equity:
            self.peak_equity = equity

    def update_pnl(self, daily: float, weekly: float):
        """Update P&L tracking."""
        self.daily_pnl = daily
        self.weekly_pnl = weekly

    def reset_kill_switch(self):
        """Manually reset the kill switch."""
        self.kill_switch_active = False
