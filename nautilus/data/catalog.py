"""TradeFox data catalog — wraps NautilusTrader ParquetDataCatalog."""

from __future__ import annotations

import os
from datetime import datetime, timezone
from pathlib import Path

from nautilus_trader.persistence.catalog import ParquetDataCatalog
from nautilus_trader.model.data import Bar, BarType
from nautilus_trader.model.identifiers import InstrumentId


class TradeFoxCatalog:
    """Manages historical data stored in Parquet format."""

    def __init__(self, path: str = "~/.tradefox/data"):
        self._path = Path(os.path.expanduser(path))
        self._path.mkdir(parents=True, exist_ok=True)
        self._catalog = ParquetDataCatalog(str(self._path))

    @property
    def path(self) -> Path:
        return self._path

    @property
    def catalog(self) -> ParquetDataCatalog:
        return self._catalog

    def list_instruments(self) -> list[str]:
        """Return list of instrument IDs with data in the catalog."""
        try:
            instruments = self._catalog.instruments()
            return [str(inst.id) for inst in instruments]
        except Exception:
            return []

    def get_date_range(
        self, instrument_id: str
    ) -> tuple[datetime, datetime] | None:
        """Return (start, end) datetime range for an instrument's data."""
        try:
            bars = self._catalog.bars(
                bar_types=[instrument_id],
            )
            if bars is None or len(bars) == 0:
                return None
            first_ts = bars[0].ts_init
            last_ts = bars[-1].ts_init
            start = datetime.fromtimestamp(first_ts / 1e9, tz=timezone.utc)
            end = datetime.fromtimestamp(last_ts / 1e9, tz=timezone.utc)
            return (start, end)
        except Exception:
            return None

    def has_data(
        self,
        instrument_id: str,
        start: datetime | None = None,
        end: datetime | None = None,
        bar_type: str | None = None,
    ) -> bool:
        """Check if the catalog has data for the given instrument."""
        try:
            instruments = self.list_instruments()
            return instrument_id in instruments
        except Exception:
            return False

    def write_bars(self, bars: list[Bar]) -> int:
        """Write bars to the catalog. Returns number written."""
        if not bars:
            return 0
        self._catalog.write_data(bars)
        return len(bars)

    def read_bars(
        self,
        bar_type: BarType,
        start: datetime | None = None,
        end: datetime | None = None,
    ) -> list[Bar]:
        """Read bars from the catalog."""
        kwargs = {}
        if start:
            kwargs["start"] = start
        if end:
            kwargs["end"] = end
        try:
            return self._catalog.bars(
                bar_types=[str(bar_type)],
                **kwargs,
            ) or []
        except Exception:
            return []

    def data_dir(self) -> Path:
        return self._path

    def instruments_dir(self) -> Path:
        d = self._path / "instruments"
        d.mkdir(parents=True, exist_ok=True)
        return d
