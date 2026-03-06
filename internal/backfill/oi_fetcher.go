package backfill

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/recorder"
)

// OIFetcher fetches historical open interest data from Binance and writes them as KiyotakaRecords.
type OIFetcher struct {
	client     *Client
	writer     *recorder.RotatingWriter
	checkpoint *CheckpointStore
	logger     *slog.Logger
}

func NewOIFetcher(client *Client, writer *recorder.RotatingWriter, cp *CheckpointStore, logger *slog.Logger) *OIFetcher {
	return &OIFetcher{client: client, writer: writer, checkpoint: cp, logger: logger}
}

// Fetch fetches all open interest history for the symbol in the given time range.
// OI data is at 5-minute intervals.
// Paginates automatically. Saves checkpoint every 10 pages.
// Returns total records written.
func (f *OIFetcher) Fetch(ctx context.Context, symbol string, tr TimeRange) (int64, error) {
	cp, _ := f.checkpoint.Load(symbol, "oi")
	startMS := tr.Start.UnixMilli()
	if cp.LastTS > startMS {
		startMS = cp.LastTS + 1
		f.logger.Info("resuming OI from checkpoint", slog.String("symbol", symbol), slog.Int64("from", startMS))
	}
	endMS := tr.End.UnixMilli()

	var total int64
	pages := 0

	for startMS < endMS {
		if ctx.Err() != nil {
			return total, ctx.Err()
		}

		records, err := f.client.FetchOIHistory(ctx, symbol, startMS, endMS)
		if err != nil {
			return total, fmt.Errorf("fetch oi %s: %w", symbol, err)
		}

		if len(records) == 0 {
			break
		}

		var lastTS int64
		for _, r := range records {
			val, _ := strconv.ParseFloat(r.SumOpenInterestValue, 64)

			ts := time.UnixMilli(r.Timestamp)
			rec := recorder.KiyotakaRecord{
				Type:      "oi",
				Symbol:    symbol,
				Exchange:  "binance",
				Timestamp: r.Timestamp,
				LocalTS:   r.Timestamp,
				Value:     val,
			}
			if err := f.writer.WriteAt(rec, ts); err != nil {
				return total, fmt.Errorf("write oi: %w", err)
			}
			total++
			lastTS = r.Timestamp
		}

		startMS = lastTS + 1

		pages++
		if pages%10 == 0 {
			_ = f.checkpoint.Save(Checkpoint{Symbol: symbol, DataType: "oi", LastTS: lastTS})
			f.logger.Info("oi progress",
				slog.String("symbol", symbol),
				slog.Int64("total", total),
				slog.String("at", time.UnixMilli(lastTS).Format("2006-01-02 15:04")),
			)
		}

		if len(records) < maxOIPerReq {
			break
		}
	}

	if total > 0 {
		_ = f.checkpoint.Save(Checkpoint{Symbol: symbol, DataType: "oi", LastTS: startMS - 1})
	}
	return total, nil
}
