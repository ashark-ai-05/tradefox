"""gRPC servicer implementations for the TradeFox Nautilus bridge."""

import json
import os
import time
import uuid
from datetime import datetime, timezone
from pathlib import Path

from .proto import nautilus_pb2, nautilus_pb2_grpc

# Strategy registry
STRATEGY_REGISTRY = {}

try:
    from .strategies.scalp_absorption import ScalpAbsorption, ScalpAbsorptionConfig
    STRATEGY_REGISTRY["scalp_absorption"] = (ScalpAbsorption, ScalpAbsorptionConfig)
except ImportError:
    pass

try:
    from .strategies.day_fvg import DayTradeFVG, DayTradeFVGConfig
    STRATEGY_REGISTRY["day_fvg"] = (DayTradeFVG, DayTradeFVGConfig)
except ImportError:
    pass

try:
    from .strategies.swing_liquidity import SwingLiquidity, SwingLiquidityConfig
    STRATEGY_REGISTRY["swing_liquidity"] = (SwingLiquidity, SwingLiquidityConfig)
except ImportError:
    pass


def _backtests_dir() -> Path:
    d = Path(os.path.expanduser("~/.tradefox/backtests"))
    d.mkdir(parents=True, exist_ok=True)
    return d


class StrategyServicer(nautilus_pb2_grpc.StrategyServiceServicer):

    def ListStrategies(self, request, context):
        return nautilus_pb2.StrategyList(strategies=[])

    def DeployStrategy(self, request, context):
        return nautilus_pb2.DeployResponse(
            ok=False, strategy_id="", message="not implemented yet",
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
        return
        yield  # noqa: unreachable


class ExecutionServicer(nautilus_pb2_grpc.ExecutionServiceServicer):

    def SubmitOrder(self, request, context):
        return nautilus_pb2.OrderResponse(
            ok=True, order_id="mock-order-001",
            client_order_id=request.client_order_id,
            status="accepted", message="mock accepted",
        )

    def CancelOrder(self, request, context):
        return nautilus_pb2.StatusResponse(ok=True, message="ok")

    def ModifyOrder(self, request, context):
        return nautilus_pb2.OrderResponse(
            ok=True, order_id=request.order_id,
            status="modified", message="mock modified",
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
            total_equity=10000.0, available_balance=10000.0,
            margin_used=0.0, currency="USDT", positions=[],
        )

    def GetPositions(self, request, context):
        return nautilus_pb2.PositionList(positions=[])

    def GetRiskMetrics(self, request, context):
        return nautilus_pb2.RiskMetrics(
            total_equity=10000.0, unrealized_pnl=0.0, realized_pnl=0.0,
            max_drawdown=0.0, daily_pnl=0.0, daily_loss_limit=300.0,
            daily_loss_used=0.0, kill_switch_active=False, positions=[],
        )

    def StreamPnL(self, request, context):
        return
        yield


class BacktestServicer(nautilus_pb2_grpc.BacktestServiceServicer):

    def RunBacktest(self, request, context):
        backtest_id = str(uuid.uuid4())[:8]

        yield nautilus_pb2.BacktestProgress(
            backtest_id=backtest_id, pct_complete=5.0,
            status="initializing", message="Setting up backtest engine...",
            timestamp_ns=time.time_ns(),
        )

        strategy_class_name = request.strategy_class
        if strategy_class_name not in STRATEGY_REGISTRY:
            yield nautilus_pb2.BacktestProgress(
                backtest_id=backtest_id, pct_complete=0.0,
                status="error",
                message=f"Unknown strategy: {strategy_class_name}. Available: {list(STRATEGY_REGISTRY.keys())}",
                timestamp_ns=time.time_ns(),
            )
            return

        try:
            from nautilus_trader.backtest.engine import BacktestEngine, BacktestEngineConfig
            from nautilus_trader.model.currencies import Currency
            from nautilus_trader.model.enums import AccountType, OmsType
            from nautilus_trader.model.identifiers import Venue
            from nautilus_trader.model.objects import Money

            from .data.catalog import TradeFoxCatalog
            from .data.instruments import fetch_binance_instrument
        except ImportError as e:
            yield nautilus_pb2.BacktestProgress(
                backtest_id=backtest_id, pct_complete=0.0,
                status="error", message=f"Import error: {e}",
                timestamp_ns=time.time_ns(),
            )
            return

        yield nautilus_pb2.BacktestProgress(
            backtest_id=backtest_id, pct_complete=10.0,
            status="loading", message="Loading data from catalog...",
            timestamp_ns=time.time_ns(),
        )

        # Parse config
        symbols = list(request.symbols) or ["BTCUSDT"]
        symbol = symbols[0]
        venue_str = request.venue or "BINANCE"

        # Parse strategy params
        params = dict(request.strategy_params)
        starting_balance = float(params.pop("starting_balance", "10000"))
        instrument_id_str = params.get("instrument_id_str", f"{symbol}-PERP.{venue_str}")
        params["instrument_id_str"] = instrument_id_str

        try:
            # Load instrument
            instrument = fetch_binance_instrument(symbol)

            # Create engine
            engine = BacktestEngine(config=BacktestEngineConfig(
                trader_id="BACKTESTER-001",
            ))

            venue = Venue(venue_str)
            engine.add_venue(
                venue=venue,
                oms_type=OmsType.NETTING,
                account_type=AccountType.MARGIN,
                base_currency=Currency.from_str("USDT"),
                starting_balances=[Money(starting_balance, Currency.from_str("USDT"))],
            )
            engine.add_instrument(instrument)

            yield nautilus_pb2.BacktestProgress(
                backtest_id=backtest_id, pct_complete=30.0,
                status="loading", message="Loading bar data...",
                timestamp_ns=time.time_ns(),
            )

            # Load data from catalog
            catalog = TradeFoxCatalog()
            from nautilus_trader.model.data import BarType
            bar_type_str = request.data_type or f"{instrument_id_str}-1-MINUTE-LAST-EXTERNAL"
            bars = catalog.read_bars(
                bar_type=BarType.from_str(bar_type_str),
            )

            if not bars:
                yield nautilus_pb2.BacktestProgress(
                    backtest_id=backtest_id, pct_complete=0.0,
                    status="error",
                    message=f"No bar data found in catalog. Import data first.",
                    timestamp_ns=time.time_ns(),
                )
                return

            engine.add_data(bars)

            yield nautilus_pb2.BacktestProgress(
                backtest_id=backtest_id, pct_complete=50.0,
                status="running",
                message=f"Running backtest with {len(bars)} bars...",
                timestamp_ns=time.time_ns(),
            )

            # Create strategy
            strategy_cls, config_cls = STRATEGY_REGISTRY[strategy_class_name]
            config = config_cls(**params)
            strategy = strategy_cls(config=config)
            engine.add_strategy(strategy)

            # Run
            engine.run()

            yield nautilus_pb2.BacktestProgress(
                backtest_id=backtest_id, pct_complete=90.0,
                status="analyzing", message="Generating reports...",
                timestamp_ns=time.time_ns(),
            )

            # Extract results
            result = self._extract_results(engine, backtest_id, strategy_class_name)

            # Save to disk
            self._save_result(backtest_id, result, strategy_class_name)

            engine.dispose()

            yield nautilus_pb2.BacktestProgress(
                backtest_id=backtest_id, pct_complete=100.0,
                status="complete", message="Backtest complete",
                timestamp_ns=time.time_ns(),
            )

        except Exception as e:
            yield nautilus_pb2.BacktestProgress(
                backtest_id=backtest_id, pct_complete=0.0,
                status="error", message=f"Backtest error: {e}",
                timestamp_ns=time.time_ns(),
            )

    def _extract_results(self, engine, backtest_id: str, strategy_class: str) -> nautilus_pb2.BacktestResult:
        """Extract results from a completed backtest engine."""
        try:
            fills = engine.trader.generate_order_fills_report()
            positions = engine.trader.generate_positions_report()
            account = engine.trader.generate_account_report(Venue("BINANCE"))
        except Exception:
            fills = None
            positions = None
            account = None

        # Compute basic stats
        total_return = 0.0
        sharpe = 0.0
        max_dd = 0.0
        win_rate = 0.0
        profit_factor = 0.0
        total_trades = 0
        equity_curve = []
        trade_records = []

        if positions is not None and len(positions) > 0:
            try:
                pnls = positions["realized_pnl"].astype(float).tolist()
                total_trades = len(pnls)
                wins = [p for p in pnls if p > 0]
                losses = [p for p in pnls if p < 0]
                win_rate = len(wins) / max(total_trades, 1)
                gross_profit = sum(wins)
                gross_loss = abs(sum(losses))
                profit_factor = gross_profit / max(gross_loss, 1e-9)
                total_return = sum(pnls) / 10000.0  # as fraction of starting balance

                # Build trade records
                for _, row in positions.iterrows():
                    trade_records.append(nautilus_pb2.TradeRecord(
                        symbol=str(row.get("instrument_id", "")),
                        side=str(row.get("side", "")),
                        entry_price=float(row.get("avg_px_open", 0)),
                        exit_price=float(row.get("avg_px_close", 0)),
                        quantity=float(row.get("peak_qty", 0)),
                        pnl=float(row.get("realized_pnl", 0)),
                        pnl_pct=float(row.get("realized_pnl", 0)) / 10000.0 * 100,
                        entry_time_ns=int(row.get("ts_opened", 0)),
                        exit_time_ns=int(row.get("ts_closed", 0)),
                        strategy_id=strategy_class,
                    ))
            except Exception:
                pass

        if account is not None and len(account) > 0:
            try:
                balances = account["total"].astype(float).tolist()
                running_max = 0.0
                for i, bal in enumerate(balances):
                    if bal > running_max:
                        running_max = bal
                    dd = (running_max - bal) / max(running_max, 1e-9)
                    if dd > max_dd:
                        max_dd = dd
                    equity_curve.append(nautilus_pb2.EquityCurvePoint(
                        timestamp_ns=i * 60_000_000_000,  # approximate
                        equity=bal,
                        drawdown=dd,
                    ))
            except Exception:
                pass

        # Sharpe approximation
        if positions is not None and total_trades > 1:
            try:
                import statistics
                pnls = positions["realized_pnl"].astype(float).tolist()
                mean_pnl = statistics.mean(pnls)
                std_pnl = statistics.stdev(pnls)
                if std_pnl > 0:
                    sharpe = (mean_pnl / std_pnl) * (252 ** 0.5)  # Annualized
            except Exception:
                pass

        return nautilus_pb2.BacktestResult(
            id=backtest_id,
            total_return=total_return,
            sharpe_ratio=sharpe,
            max_drawdown=max_dd,
            win_rate=win_rate,
            total_trades=total_trades,
            profit_factor=profit_factor,
            equity_curve=equity_curve,
            trades=trade_records,
        )

    def _save_result(self, backtest_id: str, result: nautilus_pb2.BacktestResult, strategy_class: str):
        """Save backtest result as JSON."""
        d = _backtests_dir()
        data = {
            "id": backtest_id,
            "strategy_class": strategy_class,
            "total_return": result.total_return,
            "sharpe_ratio": result.sharpe_ratio,
            "max_drawdown": result.max_drawdown,
            "win_rate": result.win_rate,
            "total_trades": result.total_trades,
            "profit_factor": result.profit_factor,
            "completed_at": datetime.now(tz=timezone.utc).isoformat(),
            "equity_curve": [
                {"ts": p.timestamp_ns, "equity": p.equity, "dd": p.drawdown}
                for p in result.equity_curve
            ],
            "trades": [
                {
                    "symbol": t.symbol, "side": t.side,
                    "entry": t.entry_price, "exit": t.exit_price,
                    "qty": t.quantity, "pnl": t.pnl, "pnl_pct": t.pnl_pct,
                }
                for t in result.trades
            ],
        }
        with open(d / f"{backtest_id}.json", "w") as f:
            json.dump(data, f, indent=2)

    def GetBacktestResult(self, request, context):
        path = _backtests_dir() / f"{request.id}.json"
        if not path.exists():
            return nautilus_pb2.BacktestResult(id=request.id)

        with open(path) as f:
            data = json.load(f)

        equity_curve = [
            nautilus_pb2.EquityCurvePoint(
                timestamp_ns=p["ts"], equity=p["equity"], drawdown=p["dd"]
            )
            for p in data.get("equity_curve", [])
        ]
        trades = [
            nautilus_pb2.TradeRecord(
                symbol=t["symbol"], side=t["side"],
                entry_price=t["entry"], exit_price=t["exit"],
                quantity=t["qty"], pnl=t["pnl"], pnl_pct=t["pnl_pct"],
            )
            for t in data.get("trades", [])
        ]

        return nautilus_pb2.BacktestResult(
            id=data["id"],
            total_return=data.get("total_return", 0),
            sharpe_ratio=data.get("sharpe_ratio", 0),
            max_drawdown=data.get("max_drawdown", 0),
            win_rate=data.get("win_rate", 0),
            total_trades=data.get("total_trades", 0),
            profit_factor=data.get("profit_factor", 0),
            equity_curve=equity_curve,
            trades=trades,
        )

    def ListBacktests(self, request, context):
        d = _backtests_dir()
        backtests = []
        for path in sorted(d.glob("*.json"), reverse=True):
            try:
                with open(path) as f:
                    data = json.load(f)
                backtests.append(nautilus_pb2.BacktestSummary(
                    id=data["id"],
                    strategy_class=data.get("strategy_class", ""),
                    status="complete",
                    total_return=data.get("total_return", 0),
                    sharpe_ratio=data.get("sharpe_ratio", 0),
                ))
            except Exception:
                continue
        return nautilus_pb2.BacktestList(backtests=backtests)

    def CompareStrategies(self, request, context):
        results = []
        for bt_id in request.backtest_ids:
            result = self.GetBacktestResult(
                nautilus_pb2.BacktestId(id=bt_id), context
            )
            results.append(result)
        return nautilus_pb2.CompareResult(results=results)


class DataServicer(nautilus_pb2_grpc.DataServiceServicer):

    def ListInstruments(self, request, context):
        try:
            from .data.catalog import TradeFoxCatalog
            catalog = TradeFoxCatalog()
            instrument_ids = catalog.list_instruments()
            instruments = []
            for iid in instrument_ids:
                parts = iid.split(".")
                venue = parts[-1] if len(parts) > 1 else ""
                symbol = parts[0] if parts else iid
                base = symbol.replace("-PERP", "").replace("USDT", "")
                instruments.append(nautilus_pb2.Instrument(
                    symbol=symbol, venue=venue,
                    instrument_type="CRYPTO_FUTURE",
                    base_currency=base, quote_currency="USDT",
                ))
            return nautilus_pb2.InstrumentList(instruments=instruments)
        except Exception:
            return nautilus_pb2.InstrumentList(instruments=[])

    def ImportData(self, request, context):
        try:
            from .data.catalog import TradeFoxCatalog
            from .data.importers import BinanceKlineImporter

            catalog = TradeFoxCatalog()
            importer = BinanceKlineImporter(catalog)

            symbol = request.symbol or "BTCUSDT"
            data_type = request.data_type or "1m"

            start_dt = None
            end_dt = None
            if request.start_ns > 0:
                start_dt = datetime.fromtimestamp(request.start_ns / 1e9, tz=timezone.utc)
            if request.end_ns > 0:
                end_dt = datetime.fromtimestamp(request.end_ns / 1e9, tz=timezone.utc)

            last_progress = [0.0]

            def on_progress(pct, msg):
                last_progress[0] = pct

            total = importer.import_klines(
                symbol=symbol,
                interval=data_type,
                start_date=start_dt or "2024-01-01",
                end_date=end_dt,
                progress_cb=on_progress,
            )

            yield nautilus_pb2.ImportProgress(
                pct_complete=100.0,
                status="complete",
                message=f"Imported {total} bars",
                records_imported=total,
            )

        except Exception as e:
            yield nautilus_pb2.ImportProgress(
                pct_complete=0.0,
                status="error",
                message=str(e),
                records_imported=0,
            )

    def GetDataRange(self, request, context):
        try:
            from .data.catalog import TradeFoxCatalog
            catalog = TradeFoxCatalog()
            instrument_id = f"{request.symbol}-PERP.{request.venue}"
            date_range = catalog.get_date_range(instrument_id)
            if date_range:
                start, end = date_range
                return nautilus_pb2.DataRange(
                    venue=request.venue,
                    symbol=request.symbol,
                    data_type=request.data_type,
                    start_ns=int(start.timestamp() * 1e9),
                    end_ns=int(end.timestamp() * 1e9),
                )
        except Exception:
            pass
        return nautilus_pb2.DataRange(
            venue=request.venue,
            symbol=request.symbol,
            data_type=request.data_type,
        )
