package backfill

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/recorder"
)

// KlineFetcher fetches historical 1-minute klines from Binance and writes them as KiyotakaRecords.
type KlineFetcher struct {
	client     *Client
	writer     *recorder.RotatingWriter
	checkpoint *CheckpointStore
	logger     *slog.Logger
}

func NewKlineFetcher(client *Client, writer *recorder.RotatingWriter, cp *CheckpointStore, logger *slog.Logger) *KlineFetcher {
	return &KlineFetcher{client: client, writer: writer, checkpoint: cp, logger: logger}
}

// Fetch fetches all 1-minute klines for the symbol in the given time range.
// Paginates automatically. Saves checkpoint every 10 pages.
// Returns total records written.
func (f *KlineFetcher) Fetch(ctx context.Context, symbol string, tr TimeRange) (int64, error) {
	cp, _ := f.checkpoint.Load(symbol, "klines")
	startMS := tr.Start.UnixMilli()
	if cp.LastTS > startMS {
		startMS = cp.LastTS + 1
	}
	endMS := tr.End.UnixMilli()

	var total int64
	pages := 0

	for startMS < endMS {
		if ctx.Err() != nil {
			return total, ctx.Err()
		}

		rawKlines, err := f.client.FetchKlines(ctx, symbol, startMS, endMS)
		if err != nil {
			return total, fmt.Errorf("fetch klines %s: %w", symbol, err)
		}

		if len(rawKlines) == 0 {
			break
		}

		var lastTS int64
		for _, raw := range rawKlines {
			// Parse Binance kline array: [openTime, open, high, low, close, volume, closeTime, ...]
			var arr []json.Number
			if err := json.Unmarshal(raw, &arr); err != nil {
				continue
			}
			if len(arr) < 7 {
				continue
			}

			openTime, _ := arr[0].Int64()
			open, _ := strconv.ParseFloat(arr[1].String(), 64)
			high, _ := strconv.ParseFloat(arr[2].String(), 64)
			low, _ := strconv.ParseFloat(arr[3].String(), 64)
			close_, _ := strconv.ParseFloat(arr[4].String(), 64)
			vol, _ := strconv.ParseFloat(arr[5].String(), 64)

			ts := time.UnixMilli(openTime)
			rec := recorder.KiyotakaRecord{
				Type:      "ohlcv",
				Symbol:    symbol,
				Exchange:  "binance",
				Timestamp: openTime,
				LocalTS:   openTime,
				Open:      open,
				High:      high,
				Low:       low,
				Close:     close_,
				Volume:    vol,
			}
			if err := f.writer.WriteAt(rec, ts); err != nil {
				return total, fmt.Errorf("write kline: %w", err)
			}
			total++
			lastTS = openTime
		}

		startMS = lastTS + 60001 // advance past this minute

		pages++
		if pages%10 == 0 {
			_ = f.checkpoint.Save(Checkpoint{Symbol: symbol, DataType: "klines", LastTS: lastTS})
			f.logger.Info("klines progress",
				slog.String("symbol", symbol),
				slog.Int64("total", total),
				slog.String("at", time.UnixMilli(lastTS).Format("2006-01-02 15:04")),
			)
		}

		if len(rawKlines) < maxKlinesPerReq {
			break
		}
	}

	if total > 0 {
		_ = f.checkpoint.Save(Checkpoint{Symbol: symbol, DataType: "klines", LastTS: startMS - 1})
	}
	return total, nil
}
