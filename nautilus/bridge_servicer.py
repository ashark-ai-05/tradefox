import time
from .proto import nautilus_pb2, nautilus_pb2_grpc


class StrategyServicer(nautilus_pb2_grpc.StrategyServiceServicer):

    def ListStrategies(self, request, context):
        return nautilus_pb2.StrategyList(strategies=[])

    def DeployStrategy(self, request, context):
        return nautilus_pb2.DeployResponse(
            ok=False,
            strategy_id="",
            message="not implemented yet",
        )

    def StopStrategy(self, request, context):
        return nautilus_pb2.StatusResponse(ok=True, message="ok")

    def GetStrategyState(self, request, context):
        return nautilus_pb2.StrategyState(
            id=request.id,
            class_name="MockStrategy",
            state="RUNNING",
            unrealized_pnl=0.0,
            realized_pnl=0.0,
            open_positions=0,
            total_signals=0,
        )

    def StreamSignals(self, request, context):
        # Empty stream — no signals in stub mode
        return
        yield  # noqa: unreachable — makes this a generator


class ExecutionServicer(nautilus_pb2_grpc.ExecutionServiceServicer):

    def SubmitOrder(self, request, context):
        return nautilus_pb2.OrderResponse(
            ok=True,
            order_id="mock-order-001",
            client_order_id=request.client_order_id,
            status="accepted",
            message="mock accepted",
        )

    def CancelOrder(self, request, context):
        return nautilus_pb2.StatusResponse(ok=True, message="ok")

    def ModifyOrder(self, request, context):
        return nautilus_pb2.OrderResponse(
            ok=True,
            order_id=request.order_id,
            status="modified",
            message="mock modified",
        )

    def StreamOrderUpdates(self, request, context):
        return
        yield

    def StreamPositionUpdates(self, request, context):
        return
        yield


class PortfolioServicer(nautilus_pb2_grpc.PortfolioServiceServicer):

    def GetPortfolio(self, request, context):
        return nautilus_pb2.Portfolio(
            total_equity=10000.0,
            available_balance=10000.0,
            margin_used=0.0,
            currency="USDT",
            positions=[],
        )

    def GetPositions(self, request, context):
        return nautilus_pb2.PositionList(positions=[])

    def GetRiskMetrics(self, request, context):
        return nautilus_pb2.RiskMetrics(
            total_equity=10000.0,
            unrealized_pnl=0.0,
            realized_pnl=0.0,
            max_drawdown=0.0,
            daily_pnl=0.0,
            daily_loss_limit=300.0,
            daily_loss_used=0.0,
            kill_switch_active=False,
            positions=[],
        )

    def StreamPnL(self, request, context):
        return
        yield


class BacktestServicer(nautilus_pb2_grpc.BacktestServiceServicer):

    def RunBacktest(self, request, context):
        yield nautilus_pb2.BacktestProgress(
            backtest_id="",
            pct_complete=0.0,
            status="not_implemented",
            message="not implemented",
            timestamp_ns=time.time_ns(),
        )

    def GetBacktestResult(self, request, context):
        return nautilus_pb2.BacktestResult(id=request.id)

    def ListBacktests(self, request, context):
        return nautilus_pb2.BacktestList(backtests=[])

    def CompareStrategies(self, request, context):
        return nautilus_pb2.CompareResult(results=[])


class DataServicer(nautilus_pb2_grpc.DataServiceServicer):

    def ListInstruments(self, request, context):
        return nautilus_pb2.InstrumentList(instruments=[])

    def ImportData(self, request, context):
        return
        yield

    def GetDataRange(self, request, context):
        return nautilus_pb2.DataRange(
            venue=request.venue,
            symbol=request.symbol,
            data_type=request.data_type,
        )
