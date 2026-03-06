"""Day Trade FVG strategy — Fair Value Gap with EMA confluence."""

from __future__ import annotations

from dataclasses import dataclass

from nautilus_trader.config import StrategyConfig
from nautilus_trader.indicators import ExponentialMovingAverage
from nautilus_trader.model.data import Bar, BarType, BarSpecification
from nautilus_trader.model.enums import (
    AggregationSource,
    BarAggregation,
    OrderSide,
    PriceType,
    TimeInForce,
)
from nautilus_trader.model.identifiers import InstrumentId
from nautilus_trader.model.objects import Price, Quantity

from .base import TradeFoxStrategy


@dataclass
class FVGZone:
    top: float
    bottom: float
    direction: str  # "bullish" or "bearish"
    bar_index: int
    filled: bool = False


class DayTradeFVGConfig(StrategyConfig, frozen=True):
    instrument_id_str: str = "BTCUSDT-PERP.BINANCE"
    fvg_min_size_pct: float = 0.001
    ema_fast: int = 9
    ema_mid: int = 21
    ema_slow: int = 50
    risk_per_trade_pct: float = 0.02


class DayTradeFVG(TradeFoxStrategy):
    """Day trading strategy using Fair Value Gaps with EMA trend confluence.

    Scans for FVG patterns on 5-minute bars:
    - Bullish FVG: bar[i-2].high < bar[i].low (gap up)
    - Bearish FVG: bar[i-2].low > bar[i].high (gap down)

    Entry when price retraces to FVG zone AND EMA stack confirms bias.
    """

    def __init__(self, config: DayTradeFVGConfig):
        super().__init__(config)
        self.instrument_id = InstrumentId.from_str(config.instrument_id_str)
        self.fvg_min_pct = config.fvg_min_size_pct
        self.risk_pct = config.risk_per_trade_pct

        # EMAs
        self._ema_fast = ExponentialMovingAverage(config.ema_fast)
        self._ema_mid = ExponentialMovingAverage(config.ema_mid)
        self._ema_slow = ExponentialMovingAverage(config.ema_slow)

        # State
        self._bars_5m: list[Bar] = []
        self._fvg_zones: list[FVGZone] = []
        self._bar_index = 0
        self._pending_order_id = None

    def on_start(self):
        instrument = self.cache.instrument(self.instrument_id)
        if instrument is None:
            self.log.error(f"Instrument {self.instrument_id} not found")
            return

        # Subscribe to 5-minute bars
        bar_spec_5m = BarSpecification(
            step=5,
            aggregation=BarAggregation.MINUTE,
            price_type=PriceType.LAST,
        )
        bar_type_5m = BarType(
            instrument_id=self.instrument_id,
            bar_spec=bar_spec_5m,
            aggregation_source=AggregationSource.INTERNAL,
        )
        self.subscribe_bars(bar_type_5m)

        # Register indicators
        self.register_indicator_for_bars(bar_type_5m, self._ema_fast)
        self.register_indicator_for_bars(bar_type_5m, self._ema_mid)
        self.register_indicator_for_bars(bar_type_5m, self._ema_slow)

    def on_bar(self, bar: Bar):
        self._bars_5m.append(bar)
        self._bar_index += 1
        
        if self._bar_index % 500 == 0:
            self.log.info(
                f"Bar #{self._bar_index}: close={bar.close}, "
                f"ema_init={self._ema_slow.initialized}, "
                f"zones={len(self._fvg_zones)}, "
                f"ema_f={self._ema_fast.value:.2f} ema_m={self._ema_mid.value:.2f} ema_s={self._ema_slow.value:.2f}"
                if self._ema_slow.initialized else
                f"Bar #{self._bar_index}: close={bar.close}, ema not initialized yet"
            )

        # Keep last 200 bars
        if len(self._bars_5m) > 200:
            self._bars_5m = self._bars_5m[-200:]

        # Need at least 3 bars for FVG detection
        if len(self._bars_5m) < 3:
            return

        # Scan for new FVG
        self._detect_fvg()

        # Clean up old/filled zones
        self._cleanup_zones()

        # Check for entry
        if not self._ema_slow.initialized:
            return

        self._check_entry(bar)

    def _detect_fvg(self):
        """Check the last 3 bars for a Fair Value Gap."""
        b0 = self._bars_5m[-3]  # bar i-2
        b2 = self._bars_5m[-1]  # bar i

        h0 = float(b0.high)
        l0 = float(b0.low)
        h2 = float(b2.high)
        l2 = float(b2.low)
        mid_price = (h0 + l0 + h2 + l2) / 4.0

        # Bullish FVG: gap between bar[i-2] high and bar[i] low
        if h0 < l2:
            gap_size = (l2 - h0) / mid_price
            if gap_size >= self.fvg_min_pct:
                self._fvg_zones.append(
                    FVGZone(
                        top=l2,
                        bottom=h0,
                        direction="bullish",
                        bar_index=self._bar_index,
                    )
                )

        # Bearish FVG: gap between bar[i-2] low and bar[i] high
        if l0 > h2:
            gap_size = (l0 - h2) / mid_price
            if gap_size >= self.fvg_min_pct:
                self._fvg_zones.append(
                    FVGZone(
                        top=l0,
                        bottom=h2,
                        direction="bearish",
                        bar_index=self._bar_index,
                    )
                )

    def _cleanup_zones(self):
        """Remove FVG zones older than 50 bars or already filled."""
        cutoff = self._bar_index - 50
        self._fvg_zones = [
            z for z in self._fvg_zones
            if not z.filled and z.bar_index > cutoff
        ]

    def _check_entry(self, bar: Bar):
        """Check if price is retracing into an FVG zone with EMA bias."""
        close = float(bar.close)

        # Determine bias from EMA stack
        ema_f = self._ema_fast.value
        ema_m = self._ema_mid.value
        ema_s = self._ema_slow.value

        bullish_bias = ema_f > ema_m > ema_s
        bearish_bias = ema_f < ema_m < ema_s

        if not bullish_bias and not bearish_bias:
            return

        # Already in position?
        if self.portfolio.is_net_long(self.instrument_id) or self.portfolio.is_net_short(self.instrument_id):
            return

        instrument = self.cache.instrument(self.instrument_id)
        if instrument is None:
            return

        for zone in self._fvg_zones:
            if zone.filled:
                continue

            midpoint = (zone.top + zone.bottom) / 2.0

            # Bullish: price retraces down into bullish FVG
            if zone.direction == "bullish" and bullish_bias:
                if zone.bottom <= close <= zone.top:
                    self._enter_trade(
                        instrument, "buy", midpoint, zone.bottom, zone.top, close
                    )
                    zone.filled = True
                    return

            # Bearish: price retraces up into bearish FVG
            if zone.direction == "bearish" and bearish_bias:
                if zone.bottom <= close <= zone.top:
                    self._enter_trade(
                        instrument, "sell", midpoint, zone.top, zone.bottom, close
                    )
                    zone.filled = True
                    return

    def _enter_trade(self, instrument, side: str, entry_price: float,
                     stop_price: float, target_price: float, current_price: float):
        """Submit entry order with stop and target."""
        account = self.portfolio.account(instrument.id.venue)
        if account is None:
            return
        equity = float(account.balance_total().as_double())

        stop_distance = abs(entry_price - stop_price)
        if stop_distance <= 0:
            return
        risk_amount = equity * self.risk_pct
        size = risk_amount / stop_distance
        size = round(size, instrument.size_precision)
        if size <= 0:
            return

        order_side = OrderSide.BUY if side == "buy" else OrderSide.SELL
        order = self.order_factory.limit(
            instrument_id=self.instrument_id,
            order_side=order_side,
            quantity=Quantity.from_str(f"{size:.{instrument.size_precision}f}"),
            price=Price.from_str(f"{entry_price:.{instrument.price_precision}f}"),
            time_in_force=TimeInForce.GTC,
            post_only=True,
        )
        self.submit_order(order)

        self.on_signal(
            signal_type="entry",
            symbol=str(self.instrument_id),
            side=side,
            price=entry_price,
            confidence=0.65,
            metadata={
                "strategy": "day_fvg",
                "stop": str(stop_price),
                "target": str(target_price),
                "ema_fast": str(round(self._ema_fast.value, 2)),
                "ema_mid": str(round(self._ema_mid.value, 2)),
                "ema_slow": str(round(self._ema_slow.value, 2)),
            },
        )

    def on_stop(self):
        self.cancel_all_orders(self.instrument_id)
        self.close_all_positions(self.instrument_id)
