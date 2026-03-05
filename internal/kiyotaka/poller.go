package kiyotaka

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/recorder"
)

// Poller periodically fetches data from Kiyotaka API and writes to disk.
type Poller struct {
	client  *Client
	symbols []SymbolConfig
	writer  *recorder.RotatingWriter
	logger  *slog.Logger
	wg      sync.WaitGroup
}

// NewPoller creates a Kiyotaka data poller.
func NewPoller(client *Client, symbols []SymbolConfig, dataDir string, logger *slog.Logger) (*Poller, error) {
	dir := filepath.Join(dataDir, "kiyotaka")
	w, err := recorder.NewRotatingWriter(dir, "kiyotaka")
	if err != nil {
		return nil, fmt.Errorf("kiyotaka poller: writer: %w", err)
	}
	return &Poller{
		client:  client,
		symbols: symbols,
		writer:  w,
		logger:  logger,
	}, nil
}

// Start begins polling at the given interval until ctx is cancelled.
func (p *Poller) Start(ctx context.Context, interval time.Duration) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Poll immediately on start
		p.pollAll(ctx)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.pollAll(ctx)
			}
		}
	}()
	p.logger.Info("kiyotaka poller started",
		slog.Duration("interval", interval),
		slog.Int("symbols", len(p.symbols)),
	)
}

// Stop waits for the poller goroutine and closes the writer.
func (p *Poller) Stop() {
	p.wg.Wait()
	p.writer.Close()
	p.logger.Info("kiyotaka poller stopped")
}

func (p *Poller) pollAll(ctx context.Context) {
	for _, sym := range p.symbols {
		if ctx.Err() != nil {
			return
		}
		p.pollSymbol(ctx, sym)
	}
}

func (p *Poller) pollSymbol(ctx context.Context, sym SymbolConfig) {
	localTS := time.Now().UnixMilli()

	// Open Interest
	if oi, err := p.client.FetchOI(ctx, sym.Exchange, sym.Pair, sym.Category); err != nil {
		p.logger.Warn("kiyotaka: fetch OI failed", slog.String("symbol", sym.Symbol), slog.Any("error", err))
	} else if oi != nil {
		if err := p.writer.Write(recorder.KiyotakaRecord{
			Type:      "oi",
			Symbol:    sym.Symbol,
			Exchange:  sym.Exchange,
			Timestamp: oi.Timestamp.UnixMilli(),
			LocalTS:   localTS,
			Value:     oi.Value,
		}); err != nil {
			p.logger.Warn("kiyotaka: write oi failed", slog.String("symbol", sym.Symbol), slog.Any("error", err))
		}
	}

	// Funding Rate
	if fr, err := p.client.FetchFundingRate(ctx, sym.Exchange, sym.Pair, sym.Category); err != nil {
		p.logger.Warn("kiyotaka: fetch funding failed", slog.String("symbol", sym.Symbol), slog.Any("error", err))
	} else if fr != nil {
		if err := p.writer.Write(recorder.KiyotakaRecord{
			Type:      "funding",
			Symbol:    sym.Symbol,
			Exchange:  sym.Exchange,
			Timestamp: fr.Timestamp.UnixMilli(),
			LocalTS:   localTS,
			Rate:      fr.Rate,
		}); err != nil {
			p.logger.Warn("kiyotaka: write funding failed", slog.String("symbol", sym.Symbol), slog.Any("error", err))
		}
	}

	// Liquidations
	if liq, err := p.client.FetchLiquidations(ctx, sym.Exchange, sym.Pair, sym.Category); err != nil {
		p.logger.Warn("kiyotaka: fetch liquidations failed", slog.String("symbol", sym.Symbol), slog.Any("error", err))
	} else if liq != nil {
		if err := p.writer.Write(recorder.KiyotakaRecord{
			Type:      "liquidation",
			Symbol:    sym.Symbol,
			Exchange:  sym.Exchange,
			Timestamp: liq.Timestamp.UnixMilli(),
			LocalTS:   localTS,
			Value:     liq.BuyVolume + liq.SellVolume,
			Side:      liquidationSide(liq),
		}); err != nil {
			p.logger.Warn("kiyotaka: write liquidation failed", slog.String("symbol", sym.Symbol), slog.Any("error", err))
		}
	}

	// OHLCV (1-minute candle)
	if candle, err := p.client.FetchOHLCV(ctx, sym.Exchange, sym.Pair, sym.Category, "1m"); err != nil {
		p.logger.Warn("kiyotaka: fetch ohlcv failed", slog.String("symbol", sym.Symbol), slog.Any("error", err))
	} else if candle != nil {
		if err := p.writer.Write(recorder.KiyotakaRecord{
			Type:      "ohlcv",
			Symbol:    sym.Symbol,
			Exchange:  sym.Exchange,
			Timestamp: candle.Timestamp.UnixMilli(),
			LocalTS:   localTS,
			Open:      candle.Open,
			High:      candle.High,
			Low:       candle.Low,
			Close:     candle.Close,
			Volume:    candle.Volume,
		}); err != nil {
			p.logger.Warn("kiyotaka: write ohlcv failed", slog.String("symbol", sym.Symbol), slog.Any("error", err))
		}
	}
}

func liquidationSide(liq *LiquidationDataPoint) string {
	if liq.BuyVolume > liq.SellVolume {
		return "long"
	}
	if liq.SellVolume > liq.BuyVolume {
		return "short"
	}
	return "mixed"
}
