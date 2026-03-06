"""Scalp Absorption strategy — detects large order absorption and trades the flip."""

from __future__ import annotations

from nautilus_trader.config import StrategyConfig
from nautilus_trader.model.data import Bar, BarType, BarSpecification, OrderBookDeltas
from nautilus_trader.model.enums import (
    AggregationSource,
    BarAggregation,
    BookType,
    OrderSide,
    PriceType,
    TimeInForce,
)
from nautilus_trader.model.identifiers import InstrumentId
from nautilus_trader.model.orders import MarketOrder
from nautilus_trader.model.objects import Quantity

from .base import TradeFoxStrategy


class ScalpAbsorptionConfig(StrategyConfig, frozen=True):
    instrument_id_str: str = "BTCUSDT-PERP.BINANCE"
    absorption_threshold_usd: float = 100_000.0
    stop_pct: float = 0.001  # 0.1%
    target_multiplier: float = 2.5
    risk_per_trade_pct: float = 0.02


class ScalpAbsorption(TradeFoxStrategy):
    """Detects absorption of large resting orders and trades the flip.

    When a large resting bid/ask (>threshold) gets consumed without price
    moving proportionally, this signals a strong counter-party absorbing
    aggression. Entry is on the flip after absorption confirmation via
    1-minute bar close.
    """

    def __init__(self, config: ScalpAbsorptionConfig):
        super().__init__(config)
        self.instrument_id = InstrumentId.from_str(config.instrument_id_str)
        self.absorption_threshold = config.absorption_threshold_usd
        self.stop_pct = config.stop_pct
        self.target_mult = config.target_multiplier
        self.risk_pct = config.risk_per_trade_pct

        # State
        self._cumulative_delta = 0.0
        self._prev_delta = 0.0
        self._absorption_detected = False
        self._absorption_side = None  # "buy" or "sell"
        self._absorption_price = 0.0
        self._bid_volume_sum = 0.0
        self._ask_volume_sum = 0.0
        self._bar_count = 0

    def on_start(self):
        instrument = self.cache.instrument(self.instrument_id)
        if instrument is None:
            self.log.error(f"Instrument {self.instrument_id} not found in cache")
            return

        # Subscribe to 1-minute bars
        bar_spec = BarSpecification(
            step=1,
            aggregation=BarAggregation.MINUTE,
            price_type=PriceType.LAST,
        )
        bar_type = BarType(
            instrument_id=self.instrument_id,
            bar_spec=bar_spec,
            aggregation_source=AggregationSource.INTERNAL,
        )
        self.subscribe_bars(bar_type)

        # Subscribe to order book deltas if available
        try:
            self.subscribe_order_book_deltas(
                instrument_id=self.instrument_id,
                book_type=BookType.L2_MBP,
            )
        except Exception:
            self.log.warning("Order book deltas not available, using bar-only mode")

    def on_order_book_deltas(self, deltas: OrderBookDeltas):
        """Track cumulative delta from order book changes."""
        for delta in deltas.deltas:
            size = float(delta.size)
            price = float(delta.price)
            volume_usd = size * price

            if delta.side == OrderSide.BUY:
                self._bid_volume_sum += volume_usd
            else:
                self._ask_volume_sum += volume_usd

        # Update cumulative delta
        self._prev_delta = self._cumulative_delta
        self._cumulative_delta = self._bid_volume_sum - self._ask_volume_sum

        # Detect absorption: heavy one-sided flow but price not moving
        if self._bid_volume_sum > self.absorption_threshold:
            delta_change = self._cumulative_delta - self._prev_delta
            if delta_change < 0:
                # Heavy buying absorbed by seller — bearish absorption
                self._absorption_detected = True
                self._absorption_side = "sell"
                self._absorption_price = float(deltas.deltas[-1].price) if deltas.deltas else 0
            self._bid_volume_sum = 0.0

        if self._ask_volume_sum > self.absorption_threshold:
            delta_change = self._cumulative_delta - self._prev_delta
            if delta_change > 0:
                # Heavy selling absorbed by buyer — bullish absorption
                self._absorption_detected = True
                self._absorption_side = "buy"
                self._absorption_price = float(deltas.deltas[-1].price) if deltas.deltas else 0
            self._ask_volume_sum = 0.0

    def on_bar(self, bar: Bar):
        self._bar_count += 1

        if not self._absorption_detected:
            return

        close = float(bar.close)
        instrument = self.cache.instrument(self.instrument_id)
        if instrument is None:
            return

        # Confirm absorption with price action
        confirmed = False
        if self._absorption_side == "buy" and close > self._absorption_price:
            confirmed = True
        elif self._absorption_side == "sell" and close < self._absorption_price:
            confirmed = True

        if not confirmed:
            # Reset after 5 bars without confirmation
            if self._bar_count > 5:
                self._absorption_detected = False
                self._bar_count = 0
            return

        # Skip if already in a position
        if self.portfolio.is_net_long(self.instrument_id) or self.portfolio.is_net_short(self.instrument_id):
            self._absorption_detected = False
            return

        # Position sizing
        account = self.portfolio.account(instrument.id.venue)
        if account is None:
            return
        equity = float(account.balance_total().as_double())
        risk_amount = equity * self.risk_pct
        stop_distance = close * self.stop_pct
        if stop_distance <= 0:
            return
        size = risk_amount / stop_distance

        # Round to instrument precision
        size = round(size, instrument.size_precision)
        if size <= 0:
            return

        # Submit market order
        side = OrderSide.BUY if self._absorption_side == "buy" else OrderSide.SELL
        order = self.order_factory.market(
            instrument_id=self.instrument_id,
            order_side=side,
            quantity=Quantity.from_str(f"{size:.{instrument.size_precision}f}"),
            time_in_force=TimeInForce.IOC,
        )
        self.submit_order(order)

        # Emit signal
        self.on_signal(
            signal_type="entry",
            symbol=str(self.instrument_id),
            side=self._absorption_side,
            price=close,
            confidence=0.7,
            metadata={"strategy": "scalp_absorption", "delta": str(self._cumulative_delta)},
        )

        # Reset
        self._absorption_detected = False
        self._bar_count = 0

    def on_stop(self):
        self.close_all_positions(self.instrument_id)
