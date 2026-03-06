package backfill

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/recorder"
)

// TradeFetcher fetches historical aggregate trades from Binance and writes them as TradeRecords.
type TradeFetcher struct {
	client     *Client
	writer     *recorder.RotatingWriter
	checkpoint *CheckpointStore
	logger     *slog.Logger
}

func NewTradeFetcher(client *Client, writer *recorder.RotatingWriter, cp *CheckpointStore, logger *slog.Logger) *TradeFetcher {
	return &TradeFetcher{client: client, writer: writer, checkpoint: cp, logger: logger}
}

// Fetch fetches all aggregate trades for the symbol in the given time range.
// Paginates automatically. Saves checkpoint every 10 pages.
// Returns total records written.
func (f *TradeFetcher) Fetch(ctx context.Context, symbol string, tr TimeRange) (int64, error) {
	// Check checkpoint for resume
	cp, _ := f.checkpoint.Load(symbol, "trades")
	startMS := tr.Start.UnixMilli()
	if cp.LastTS > startMS {
		startMS = cp.LastTS + 1
		f.logger.Info("resuming trades from checkpoint", slog.String("symbol", symbol), slog.Int64("from", startMS))
	}
	endMS := tr.End.UnixMilli()

	var total int64
	pages := 0

	for startMS < endMS {
		if ctx.Err() != nil {
			return total, ctx.Err()
		}

		trades, err := f.client.FetchAggTrades(ctx, symbol, startMS, endMS)
		if err != nil {
			return total, fmt.Errorf("fetch trades %s: %w", symbol, err)
		}

		if len(trades) == 0 {
			break
		}

		for _, t := range trades {
			ts := time.UnixMilli(t.Time)
			isBuy := !t.IsMaker // in Binance, m=true means seller is maker, so buyer is taker (it's a buy)
			rec := recorder.TradeRecord{
				Type:       "trade",
				Symbol:     symbol,
				Provider:   "binance",
				Price:      t.Price,
				Size:       t.Qty,
				ExchangeTS: t.Time,
				LocalTS:    t.Time, // for historical data, local_ts = exchange_ts
				IsBuy:      &isBuy,
			}
			if err := f.writer.WriteAt(rec, ts); err != nil {
				return total, fmt.Errorf("write trade: %w", err)
			}
			total++
		}

		// Advance past the last trade
		lastTS := trades[len(trades)-1].Time
		startMS = lastTS + 1

		pages++
		if pages%10 == 0 {
			_ = f.checkpoint.Save(Checkpoint{
				Symbol:   symbol,
				DataType: "trades",
				LastTS:   lastTS,
			})
			f.logger.Info("trades progress",
				slog.String("symbol", symbol),
				slog.Int64("total", total),
				slog.String("at", time.UnixMilli(lastTS).Format("2006-01-02 15:04")),
			)
		}

		// If we got fewer than the limit, we've reached the end
		if len(trades) < maxTradesPerReq {
			break
		}
	}

	// Final checkpoint
	if total > 0 {
		_ = f.checkpoint.Save(Checkpoint{
			Symbol:   symbol,
			DataType: "trades",
			LastTS:   startMS - 1,
		})
	}

	return total, nil
}
