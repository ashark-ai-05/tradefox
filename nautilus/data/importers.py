"""Binance historical data importers for NautilusTrader."""

from __future__ import annotations

import json
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Callable
from urllib.request import urlopen, Request

from nautilus_trader.model.data import Bar, BarType, BarSpecification
from nautilus_trader.model.enums import AggregationSource, BarAggregation, PriceType
from nautilus_trader.model.identifiers import InstrumentId
from nautilus_trader.model.objects import Price, Quantity

from .catalog import TradeFoxCatalog


BINANCE_KLINE_URL = "https://fapi.binance.com/fapi/v1/klines"

INTERVAL_TO_BAR_AGG = {
    "1m": BarAggregation.MINUTE,
    "5m": BarAggregation.MINUTE,
    "15m": BarAggregation.MINUTE,
    "1h": BarAggregation.HOUR,
    "4h": BarAggregation.HOUR,
    "1d": BarAggregation.DAY,
}

INTERVAL_TO_STEP = {
    "1m": 1,
    "5m": 5,
    "15m": 15,
    "1h": 1,
    "4h": 4,
    "1d": 1,
}


class BinanceKlineImporter:
    """Imports Binance Futures kline data into the TradeFox catalog."""

    def __init__(self, catalog: TradeFoxCatalog):
        self._catalog = catalog

    def import_klines(
        self,
        symbol: str,
        interval: str = "1m",
        start_date: str | datetime = "2024-01-01",
        end_date: str | datetime | None = None,
        progress_cb: Callable[[float, str], None] | None = None,
    ) -> int:
        """Import klines from Binance Futures API.

        Args:
            symbol: e.g. "BTCUSDT"
            interval: e.g. "1m", "5m", "15m", "1h", "4h", "1d"
            start_date: Start date string or datetime
            end_date: End date string or datetime (default: now)
            progress_cb: Callback(pct, message) for progress updates

        Returns:
            Number of bars imported.
        """
        if isinstance(start_date, str):
            start_date = datetime.fromisoformat(start_date).replace(tzinfo=timezone.utc)
        if end_date is None:
            end_date = datetime.now(tz=timezone.utc)
        elif isinstance(end_date, str):
            end_date = datetime.fromisoformat(end_date).replace(tzinfo=timezone.utc)

        start_ms = int(start_date.timestamp() * 1000)
        end_ms = int(end_date.timestamp() * 1000)

        instrument_id = InstrumentId.from_str(f"{symbol}-PERP.BINANCE")
        agg = INTERVAL_TO_BAR_AGG.get(interval, BarAggregation.MINUTE)
        step = INTERVAL_TO_STEP.get(interval, 1)
        bar_spec = BarSpecification(
            step=step,
            aggregation=agg,
            price_type=PriceType.LAST,
        )
        bar_type = BarType(
            instrument_id=instrument_id,
            bar_spec=bar_spec,
            aggregation_source=AggregationSource.EXTERNAL,
        )

        total_bars = 0
        current_ms = start_ms
        total_range = end_ms - start_ms
        batch_num = 0

        while current_ms < end_ms:
            url = (
                f"{BINANCE_KLINE_URL}?symbol={symbol}"
                f"&interval={interval}&limit=1500"
                f"&startTime={current_ms}&endTime={end_ms}"
            )
            req = Request(url, headers={"User-Agent": "TradeFox/1.0"})

            try:
                with urlopen(req, timeout=30) as resp:
                    raw = json.loads(resp.read().decode())
            except Exception as e:
                if progress_cb:
                    progress_cb(
                        min(99.0, (current_ms - start_ms) / max(total_range, 1) * 100),
                        f"Error fetching: {e}, retrying...",
                    )
                time.sleep(1.0)
                continue

            if not raw:
                break

            bars = []
            for k in raw:
                # [openTime, open, high, low, close, volume, closeTime, ...]
                open_time_ns = int(k[0]) * 1_000_000  # ms -> ns
                close_time_ns = int(k[6]) * 1_000_000

                bar = Bar(
                    bar_type=bar_type,
                    open=Price.from_str(str(k[1])),
                    high=Price.from_str(str(k[2])),
                    low=Price.from_str(str(k[3])),
                    close=Price.from_str(str(k[4])),
                    volume=Quantity.from_str(str(k[5])),
                    ts_event=close_time_ns,
                    ts_init=open_time_ns,
                )
                bars.append(bar)

            if bars:
                self._catalog.write_bars(bars)
                total_bars += len(bars)

            # Move past the last kline
            last_open_ms = int(raw[-1][0])
            if last_open_ms <= current_ms:
                break
            current_ms = last_open_ms + 1

            batch_num += 1
            if progress_cb and batch_num % 1 == 0:
                pct = min(99.0, (current_ms - start_ms) / max(total_range, 1) * 100)
                progress_cb(pct, f"Imported {total_bars} bars...")

            time.sleep(0.1)  # Rate limit

        if progress_cb:
            progress_cb(100.0, f"Complete: {total_bars} bars imported")

        return total_bars
