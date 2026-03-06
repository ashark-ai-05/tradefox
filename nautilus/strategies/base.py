from nautilus_trader.trading import Strategy


class TradeFoxStrategy(Strategy):
    """Base class for TradeFox strategies.

    Emits signals via gRPC to TradeFox for visualization.
    Strategies run identically in backtest and live mode.
    """

    def __init__(self, config=None):
        super().__init__(config)
        self._bridge = None

    def set_bridge(self, bridge):
        self._bridge = bridge

    def on_signal(
        self,
        signal_type: str,
        symbol: str,
        side: str,
        price: float,
        confidence: float,
        metadata: dict | None = None,
    ):
        """Emit a signal to TradeFox via gRPC bridge."""
        if self._bridge is None:
            return
        from ..proto import nautilus_pb2

        self._bridge.emit_signal(
            nautilus_pb2.Signal(
                strategy_id=self.id.value,
                signal_type=signal_type,
                symbol=symbol,
                side=side,
                price=price,
                confidence=confidence,
                timestamp_ns=self.clock.timestamp_ns(),
                metadata=metadata or {},
            )
        )

    def on_bar(self, bar):
        """Override in subclass — your strategy logic here."""
        raise NotImplementedError
