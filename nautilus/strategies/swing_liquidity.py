"""Swing Liquidity strategy — liquidity sweep + structure break."""

from __future__ import annotations

from dataclasses import dataclass, field

from nautilus_trader.config import StrategyConfig
from nautilus_trader.model.data import Bar, BarType, BarSpecification
from nautilus_trader.model.enums import (
    AggregationSource,
    BarAggregation,
    OrderSide,
    PriceType,
    TimeInForce,
)
from nautilus_trader.model.identifiers import InstrumentId
from nautilus_trader.model.objects import Quantity

from .base import TradeFoxStrategy


@dataclass
class LiquidityLevel:
    price: float
    level_type: str  # "high" or "low"
    touches: int = 1
    last_touch_index: int = 0
    swept: bool = False
    sweep_wick: float = 0.0  # Farthest point past the level during sweep


class SwingLiquidityConfig(StrategyConfig, frozen=True):
    instrument_id_str: str = "BTCUSDT-PERP.BINANCE"
    liquidity_lookback: int = 50
    equal_level_tolerance_pct: float = 0.001
    min_touches: int = 2
    risk_per_trade_pct: float = 0.02


class SwingLiquidity(TradeFoxStrategy):
    """Swing trading strategy targeting liquidity sweeps and structure breaks.

    Process:
    1. On 4h bars: scan for equal highs/lows (liquidity levels)
    2. On 1h bars: detect sweeps (price crosses level then closes back)
    3. On 15m bars: after sweep, detect structure break for entry

    Entry: market order on structure break confirmation after sweep.
    Stop: beyond the sweep wick.
    Target: opposite side liquidity level.
    """

    def __init__(self, config: SwingLiquidityConfig):
        super().__init__(config)
        self.instrument_id = InstrumentId.from_str(config.instrument_id_str)
        self.lookback = config.liquidity_lookback
        self.tolerance = config.equal_level_tolerance_pct
        self.min_touches = config.min_touches
        self.risk_pct = config.risk_per_trade_pct

        # State
        self._liq_levels: list[LiquidityLevel] = []
        self._bars_4h: list[Bar] = []
        self._bars_1h: list[Bar] = []
        self._bars_15m: list[Bar] = []
        self._bar_index_4h = 0
        self._bar_index_1h = 0
        self._bar_index_15m = 0
        self._sweep_pending: LiquidityLevel | None = None
        self._sweep_direction: str | None = None  # "bullish" or "bearish"

        # Structure tracking for 15m
        self._recent_highs_15m: list[float] = []
        self._recent_lows_15m: list[float] = []

    def on_start(self):
        instrument = self.cache.instrument(self.instrument_id)
        if instrument is None:
            self.log.error(f"Instrument {self.instrument_id} not found")
            return

        # Subscribe to multiple timeframes
        for step, agg in [(15, BarAggregation.MINUTE), (1, BarAggregation.HOUR), (4, BarAggregation.HOUR)]:
            bar_spec = BarSpecification(step=step, aggregation=agg, price_type=PriceType.LAST)
            bar_type = BarType(
                instrument_id=self.instrument_id,
                bar_spec=bar_spec,
                aggregation_source=AggregationSource.INTERNAL,
            )
            self.subscribe_bars(bar_type)

    def on_bar(self, bar: Bar):
        """Route bar to appropriate handler based on timeframe."""
        bar_str = str(bar.bar_type.spec)

        if "4-HOUR" in bar_str:
            self._on_bar_4h(bar)
        elif "1-HOUR" in bar_str and "4-HOUR" not in bar_str:
            self._on_bar_1h(bar)
        elif "15-MINUTE" in bar_str:
            self._on_bar_15m(bar)
        else:
            # Fallback: treat all bars as 15m for backtest compatibility
            self._on_bar_15m(bar)

    def _on_bar_4h(self, bar: Bar):
        """Scan for equal highs/lows to identify liquidity levels."""
        self._bars_4h.append(bar)
        self._bar_index_4h += 1

        if len(self._bars_4h) > self.lookback * 2:
            self._bars_4h = self._bars_4h[-self.lookback * 2:]

        if len(self._bars_4h) < 3:
            return

        high = float(bar.high)
        low = float(bar.low)

        # Check for equal highs
        self._update_level(high, "high", self._bar_index_4h)
        # Check for equal lows
        self._update_level(low, "low", self._bar_index_4h)

        # Clean up old levels
        cutoff = self._bar_index_4h - self.lookback
        self._liq_levels = [
            lv for lv in self._liq_levels
            if lv.last_touch_index > cutoff
        ]

    def _update_level(self, price: float, level_type: str, bar_idx: int):
        """Add or update a liquidity level."""
        for lv in self._liq_levels:
            if lv.level_type != level_type or lv.swept:
                continue
            if abs(lv.price - price) / max(price, 1.0) <= self.tolerance:
                lv.touches += 1
                lv.last_touch_index = bar_idx
                lv.price = (lv.price + price) / 2.0  # Average the level
                return

        self._liq_levels.append(
            LiquidityLevel(
                price=price,
                level_type=level_type,
                touches=1,
                last_touch_index=bar_idx,
            )
        )

    def _on_bar_1h(self, bar: Bar):
        """Detect liquidity sweeps on 1h timeframe."""
        self._bars_1h.append(bar)
        self._bar_index_1h += 1

        if len(self._bars_1h) > 100:
            self._bars_1h = self._bars_1h[-100:]

        high = float(bar.high)
        low = float(bar.low)
        close = float(bar.close)

        for lv in self._liq_levels:
            if lv.swept or lv.touches < self.min_touches:
                continue

            # Sweep of highs: price goes above then closes back below
            if lv.level_type == "high" and high > lv.price and close < lv.price:
                lv.swept = True
                lv.sweep_wick = high
                self._sweep_pending = lv
                self._sweep_direction = "bearish"  # Swept highs = bearish

                self.on_signal(
                    signal_type="sweep",
                    symbol=str(self.instrument_id),
                    side="sell",
                    price=lv.price,
                    confidence=0.5,
                    metadata={"strategy": "swing_liquidity", "level": str(lv.price), "touches": str(lv.touches)},
                )

            # Sweep of lows: price goes below then closes back above
            if lv.level_type == "low" and low < lv.price and close > lv.price:
                lv.swept = True
                lv.sweep_wick = low
                self._sweep_pending = lv
                self._sweep_direction = "bullish"  # Swept lows = bullish

                self.on_signal(
                    signal_type="sweep",
                    symbol=str(self.instrument_id),
                    side="buy",
                    price=lv.price,
                    confidence=0.5,
                    metadata={"strategy": "swing_liquidity", "level": str(lv.price), "touches": str(lv.touches)},
                )

    def _on_bar_15m(self, bar: Bar):
        """Detect structure break after sweep for entry."""
        self._bars_15m.append(bar)
        self._bar_index_15m += 1

        if len(self._bars_15m) > 100:
            self._bars_15m = self._bars_15m[-100:]

        high = float(bar.high)
        low = float(bar.low)
        close = float(bar.close)

        # Track swing highs/lows
        self._recent_highs_15m.append(high)
        self._recent_lows_15m.append(low)
        if len(self._recent_highs_15m) > 20:
            self._recent_highs_15m = self._recent_highs_15m[-20:]
            self._recent_lows_15m = self._recent_lows_15m[-20:]

        if self._sweep_pending is None or len(self._recent_highs_15m) < 5:
            return

        # Already in position?
        if self.portfolio.is_net_long(self.instrument_id) or self.portfolio.is_net_short(self.instrument_id):
            return

        instrument = self.cache.instrument(self.instrument_id)
        if instrument is None:
            return

        # Detect structure break
        prev_highs = self._recent_highs_15m[-6:-1]
        prev_lows = self._recent_lows_15m[-6:-1]
        prev_high = max(prev_highs)
        prev_low = min(prev_lows)

        if self._sweep_direction == "bullish":
            # Bullish structure break: higher low then break above previous high
            recent_low = min(self._recent_lows_15m[-3:])
            if recent_low > prev_low and close > prev_high:
                self._enter_swing(instrument, "buy", close)
                self._sweep_pending = None

        elif self._sweep_direction == "bearish":
            # Bearish structure break: lower high then break below previous low
            recent_high = max(self._recent_highs_15m[-3:])
            if recent_high < prev_high and close < prev_low:
                self._enter_swing(instrument, "sell", close)
                self._sweep_pending = None

    def _enter_swing(self, instrument, side: str, entry_price: float):
        """Submit swing trade entry."""
        if self._sweep_pending is None:
            return

        # Stop beyond the sweep wick
        if side == "buy":
            stop_price = self._sweep_pending.sweep_wick
            stop_distance = entry_price - stop_price
        else:
            stop_price = self._sweep_pending.sweep_wick
            stop_distance = stop_price - entry_price

        if stop_distance <= 0:
            return

        # Find target: opposite side liquidity level
        target_price = entry_price
        for lv in self._liq_levels:
            if lv.swept:
                continue
            if side == "buy" and lv.level_type == "high" and lv.price > entry_price:
                target_price = lv.price
                break
            if side == "sell" and lv.level_type == "low" and lv.price < entry_price:
                target_price = lv.price
                break

        # Default target if none found
        if target_price == entry_price:
            target_price = entry_price + stop_distance * 3 if side == "buy" else entry_price - stop_distance * 3

        # Position sizing
        account = self.portfolio.account(instrument.id.venue)
        if account is None:
            return
        equity = float(account.balance_total().as_double())
        risk_amount = equity * self.risk_pct
        size = risk_amount / stop_distance
        size = round(size, instrument.size_precision)
        if size <= 0:
            return

        order_side = OrderSide.BUY if side == "buy" else OrderSide.SELL
        order = self.order_factory.market(
            instrument_id=self.instrument_id,
            order_side=order_side,
            quantity=Quantity.from_str(f"{size:.{instrument.size_precision}f}"),
            time_in_force=TimeInForce.GTC,
        )
        self.submit_order(order)

        self.on_signal(
            signal_type="entry",
            symbol=str(self.instrument_id),
            side=side,
            price=entry_price,
            confidence=0.6,
            metadata={
                "strategy": "swing_liquidity",
                "stop": str(round(stop_price, 2)),
                "target": str(round(target_price, 2)),
                "sweep_level": str(round(self._sweep_pending.price, 2)),
            },
        )

    def on_stop(self):
        self.cancel_all_orders(self.instrument_id)
        self.close_all_positions(self.instrument_id)
