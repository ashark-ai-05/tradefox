package backfill

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/recorder"
)

// FundingFetcher fetches historical funding rates from Binance and writes them as KiyotakaRecords.
type FundingFetcher struct {
	client     *Client
	writer     *recorder.RotatingWriter
	checkpoint *CheckpointStore
	logger     *slog.Logger
}

func NewFundingFetcher(client *Client, writer *recorder.RotatingWriter, cp *CheckpointStore, logger *slog.Logger) *FundingFetcher {
	return &FundingFetcher{client: client, writer: writer, checkpoint: cp, logger: logger}
}

// Fetch fetches all funding rate records for the symbol in the given time range.
// Funding rates are typically every 8 hours, so much less data than trades.
// Paginates automatically. Saves checkpoint every 10 pages.
// Returns total records written.
func (f *FundingFetcher) Fetch(ctx context.Context, symbol string, tr TimeRange) (int64, error) {
	cp, _ := f.checkpoint.Load(symbol, "funding")
	startMS := tr.Start.UnixMilli()
	if cp.LastTS > startMS {
		startMS = cp.LastTS + 1
		f.logger.Info("resuming funding from checkpoint", slog.String("symbol", symbol), slog.Int64("from", startMS))
	}
	endMS := tr.End.UnixMilli()

	var total int64
	pages := 0

	for startMS < endMS {
		if ctx.Err() != nil {
			return total, ctx.Err()
		}

		records, err := f.client.FetchFundingHistory(ctx, symbol, startMS, endMS)
		if err != nil {
			return total, fmt.Errorf("fetch funding %s: %w", symbol, err)
		}

		if len(records) == 0 {
			break
		}

		var lastTS int64
		for _, r := range records {
			rate, _ := strconv.ParseFloat(r.FundingRate, 64)

			ts := time.UnixMilli(r.FundingTime)
			rec := recorder.KiyotakaRecord{
				Type:      "funding",
				Symbol:    symbol,
				Exchange:  "binance",
				Timestamp: r.FundingTime,
				LocalTS:   r.FundingTime,
				Rate:      rate,
			}
			if err := f.writer.WriteAt(rec, ts); err != nil {
				return total, fmt.Errorf("write funding: %w", err)
			}
			total++
			lastTS = r.FundingTime
		}

		startMS = lastTS + 1

		pages++
		if pages%10 == 0 {
			_ = f.checkpoint.Save(Checkpoint{Symbol: symbol, DataType: "funding", LastTS: lastTS})
			f.logger.Info("funding progress",
				slog.String("symbol", symbol),
				slog.Int64("total", total),
				slog.String("at", time.UnixMilli(lastTS).Format("2006-01-02 15:04")),
			)
		}

		if len(records) < maxFundingPerReq {
			break
		}
	}

	if total > 0 {
		_ = f.checkpoint.Save(Checkpoint{Symbol: symbol, DataType: "funding", LastTS: startMS - 1})
	}
	return total, nil
}
