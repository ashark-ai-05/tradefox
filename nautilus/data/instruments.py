"""Binance instrument definitions for NautilusTrader."""

from __future__ import annotations

import json
import os
from pathlib import Path
from urllib.request import urlopen, Request

from nautilus_trader.model.identifiers import InstrumentId, Symbol, Venue
from nautilus_trader.model.instruments import CryptoFuture
from nautilus_trader.model.currencies import Currency
from nautilus_trader.model.objects import Money, Price, Quantity


BINANCE_EXCHANGE_INFO_URL = "https://fapi.binance.com/fapi/v1/exchangeInfo"

_CACHE_DIR = Path(os.path.expanduser("~/.tradefox/data/instruments"))


def fetch_binance_instrument(symbol: str) -> CryptoFuture:
    """Fetch instrument definition from Binance and create a CryptoFuture.

    Results are cached locally in ~/.tradefox/data/instruments/.
    """
    _CACHE_DIR.mkdir(parents=True, exist_ok=True)
    cache_path = _CACHE_DIR / f"{symbol}.json"

    # Try cache first
    if cache_path.exists():
        with open(cache_path) as f:
            info = json.load(f)
        return _build_instrument(symbol, info)

    # Fetch from Binance
    url = BINANCE_EXCHANGE_INFO_URL
    req = Request(url, headers={"User-Agent": "TradeFox/1.0"})
    with urlopen(req, timeout=30) as resp:
        data = json.loads(resp.read().decode())

    # Find our symbol
    info = None
    for s in data.get("symbols", []):
        if s["symbol"] == symbol:
            info = s
            break

    if info is None:
        raise ValueError(f"Symbol {symbol} not found on Binance Futures")

    # Cache it
    with open(cache_path, "w") as f:
        json.dump(info, f, indent=2)

    return _build_instrument(symbol, info)


def _build_instrument(symbol: str, info: dict) -> CryptoFuture:
    """Build a CryptoFuture from Binance exchange info."""
    # Extract filter values
    tick_size = "0.01"
    lot_size = "0.001"
    min_notional = "5.0"

    for f in info.get("filters", []):
        if f["filterType"] == "PRICE_FILTER":
            tick_size = f["tickSize"]
        elif f["filterType"] == "LOT_SIZE":
            lot_size = f["stepSize"]
        elif f["filterType"] == "MIN_NOTIONAL":
            min_notional = f.get("notional", "5.0")

    base = info.get("baseAsset", symbol.replace("USDT", ""))
    quote = info.get("quoteAsset", "USDT")
    settlement = info.get("marginAsset", "USDT")

    instrument_id = InstrumentId(
        symbol=Symbol(f"{symbol}-PERP"),
        venue=Venue("BINANCE"),
    )

    # Determine price/size precision from tick/lot strings
    price_precision = _precision_from_str(tick_size)
    size_precision = _precision_from_str(lot_size)

    return CryptoFuture(
        instrument_id=instrument_id,
        raw_symbol=Symbol(symbol),
        underlying=Currency.from_str(base),
        quote_currency=Currency.from_str(quote),
        settlement_currency=Currency.from_str(settlement),
        is_inverse=False,
        activation_ns=0,
        expiration_ns=0,
        price_precision=price_precision,
        size_precision=size_precision,
        price_increment=Price.from_str(tick_size),
        size_increment=Quantity.from_str(lot_size),
        max_quantity=Quantity.from_str("1000.0"),
        min_quantity=Quantity.from_str(lot_size),
        max_notional=None,
        min_notional=Money.from_str(f"{min_notional} {quote}"),
        max_price=Price.from_str("1000000.0"),
        min_price=Price.from_str(tick_size),
        margin_init=Quantity.from_str("0.05"),
        margin_maint=Quantity.from_str("0.025"),
        maker_fee=Quantity.from_str("0.0002"),
        taker_fee=Quantity.from_str("0.0004"),
        ts_event=0,
        ts_init=0,
    )


def _precision_from_str(value: str) -> int:
    """Determine decimal precision from a string like '0.01' -> 2."""
    value = value.rstrip("0")
    if "." not in value:
        return 0
    return len(value.split(".")[1])
