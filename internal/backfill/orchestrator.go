package backfill

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/recorder"
)

// Config holds backfill configuration.
type Config struct {
	Symbols []string
	DataDir string
	Start   time.Time
	End     time.Time
}

// Orchestrator coordinates backfill of all data types for all symbols.
type Orchestrator struct {
	config Config
	client *Client
	logger *slog.Logger
}

func NewOrchestrator(cfg Config, logger *slog.Logger) *Orchestrator {
	return &Orchestrator{
		config: cfg,
		client: NewClient(logger),
		logger: logger,
	}
}

// Run executes the backfill for all symbols and data types sequentially.
// For each symbol, it runs: trades, klines, funding, OI.
func (o *Orchestrator) Run(ctx context.Context) error {
	tr := TimeRange{Start: o.config.Start, End: o.config.End}

	for _, symbol := range o.config.Symbols {
		o.logger.Info("backfilling symbol", slog.String("symbol", symbol))

		cp, err := NewCheckpointStore(o.config.DataDir)
		if err != nil {
			return err
		}

		// Trades
		tradeDir := filepath.Join(o.config.DataDir, "trades")
		tradeWriter, err := recorder.NewRotatingWriter(tradeDir, "trade")
		if err != nil {
			return fmt.Errorf("trade writer: %w", err)
		}
		tf := NewTradeFetcher(o.client, tradeWriter, cp, o.logger)
		n, err := tf.Fetch(ctx, symbol, tr)
		tradeWriter.Close()
		if err != nil {
			return fmt.Errorf("trades %s: %w", symbol, err)
		}
		o.logger.Info("trades done", slog.String("symbol", symbol), slog.Int64("records", n))

		// Klines
		kiyDir := filepath.Join(o.config.DataDir, "kiyotaka")
		klineWriter, err := recorder.NewRotatingWriter(kiyDir, "kiyotaka")
		if err != nil {
			return fmt.Errorf("kline writer: %w", err)
		}
		kf := NewKlineFetcher(o.client, klineWriter, cp, o.logger)
		n, err = kf.Fetch(ctx, symbol, tr)
		klineWriter.Close()
		if err != nil {
			return fmt.Errorf("klines %s: %w", symbol, err)
		}
		o.logger.Info("klines done", slog.String("symbol", symbol), slog.Int64("records", n))

		// Funding
		fundWriter, err := recorder.NewRotatingWriter(kiyDir, "kiyotaka")
		if err != nil {
			return fmt.Errorf("funding writer: %w", err)
		}
		ff := NewFundingFetcher(o.client, fundWriter, cp, o.logger)
		n, err = ff.Fetch(ctx, symbol, tr)
		fundWriter.Close()
		if err != nil {
			return fmt.Errorf("funding %s: %w", symbol, err)
		}
		o.logger.Info("funding done", slog.String("symbol", symbol), slog.Int64("records", n))

		// OI
		oiWriter, err := recorder.NewRotatingWriter(kiyDir, "kiyotaka")
		if err != nil {
			return fmt.Errorf("oi writer: %w", err)
		}
		oif := NewOIFetcher(o.client, oiWriter, cp, o.logger)
		n, err = oif.Fetch(ctx, symbol, tr)
		oiWriter.Close()
		if err != nil {
			return fmt.Errorf("oi %s: %w", symbol, err)
		}
		o.logger.Info("oi done", slog.String("symbol", symbol), slog.Int64("records", n))
	}

	return nil
}
