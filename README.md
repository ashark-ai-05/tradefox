# TradeFox

A professional crypto trading terminal built as a TUI (Terminal User Interface) in Go.

![TradeFox Screenshot](docs/screenshot.png)

## Features

### Phase 1 - TUI Shell
- Multi-tab interface: Trading, Scanner, Positions, Settings
- Order book depth ladder with cumulative volume bars
- Watchlist with 20+ crypto pairs, real-time style data
- Position management with PnL tracking
- Scanner view with sortable columns and live filtering
- Order entry modal (Market/Limit)
- Help overlay with keyboard shortcuts
- Dark theme with professional color scheme

### Phase 2 - Charts & Signals
- **Braille candlestick chart** with Unicode block rendering
  - Green/red candle bodies with wick rendering
  - EMA(9) and EMA(21) overlay lines (cyan/yellow)
  - Volume bars at bottom with proportional block characters
  - Y-axis price labels, auto-scaled to visible range
  - Crosshair mode with OHLCV readout
- **6 timeframes**: 1m, 5m, 15m, 1h, 4h, 1D
- **Signal dashboard**: 8 microstructure signals in a 2x4 gauge grid
  - Microprice, OFI, Sweep, Depth Imbalance, Kyle's Lambda, Volatility, Spoof Detection, Composite
  - Color-coded bullish/bearish/neutral with visual gauge bars
- **Recent trades feed**: Time & sales tape with color-coded buy/sell, large trade highlighting
- **Liquidation heatmap**: Vertical ASCII heatmap showing estimated liquidation clusters
  - Warm colors (long liquidations below price)
  - Cool colors (short liquidations above price)
  - Current price marker

## Installation

```bash
go install github.com/ashark-ai-05/tradefox/cmd/tradefox@latest
```

Or build from source:

```bash
git clone https://github.com/ashark-ai-05/tradefox.git
cd tradefox
go build -o tradefox ./cmd/tradefox/...
./tradefox
```

## Usage

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `1-4` | Switch tabs (Trading, Scanner, Positions, Settings) |
| `Tab` / `Shift+Tab` | Cycle tabs |
| `j/k` or `Up/Down` | Navigate lists / move crosshair |
| `b` | Buy order entry |
| `s` | Sell order entry |
| `[` / `]` | Decrease/increase order book depth |
| `<` / `>` | Previous/next chart timeframe |
| `c` | Toggle chart crosshair |
| `Left/Right` | Move crosshair along candles |
| `l` | Toggle liquidation heatmap |
| `f` | Filter (scanner tab) |
| `?` | Help overlay |
| `q` / `Ctrl+C` | Quit |

## Architecture

TradeFox reuses backend infrastructure from the VisualHFT-Go project:

- `internal/connector/` - Exchange WebSocket connectors (Binance, Bybit, OKX)
- `internal/scanner/` - Market scanner with technical indicators
- `internal/execution/` - Order execution and position management
- `internal/pool/` - WebSocket connection pooling
- `internal/recorder/` - Trade data recording
- `internal/replay/` - Historical data replay
- `internal/backfill/` - OHLCV data backfilling
- `internal/walkforward/` - Walk-forward optimization
- `internal/tui/` - Terminal UI (BubbleTea + Lipgloss)

## Roadmap

- [x] **Phase 1** - TUI shell, order book, watchlist, positions, scanner
- [x] **Phase 2** - Braille charts, signal dashboard, liquidation heatmap, trades feed
- [ ] **Phase 3** - Live exchange integration, real-time data streaming, order execution

## License

MIT
