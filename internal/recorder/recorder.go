package recorder

import (
	"context"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

// Recorder subscribes to the event bus and writes order book snapshots and
// trades to rotating gzip-compressed JSONL files on disk.
type Recorder struct {
	bus         *eventbus.Bus
	logger      *slog.Logger
	dir         string
	obWriter    *RotatingWriter
	tradeWriter *RotatingWriter
	obSubID     uint64
	tradeSubID  uint64
	wg          sync.WaitGroup

	obCount    atomic.Int64
	tradeCount atomic.Int64
}

// New creates a Recorder that writes to subdirectories under dir.
// It creates two RotatingWriters: one in {dir}/orderbooks/ (prefix "ob")
// and one in {dir}/trades/ (prefix "trade").
func New(bus *eventbus.Bus, dir string, logger *slog.Logger) (*Recorder, error) {
	obWriter, err := NewRotatingWriter(filepath.Join(dir, "orderbooks"), "ob")
	if err != nil {
		return nil, err
	}

	tradeWriter, err := NewRotatingWriter(filepath.Join(dir, "trades"), "trade")
	if err != nil {
		_ = obWriter.Close()
		return nil, err
	}

	return &Recorder{
		bus:         bus,
		logger:      logger,
		dir:         dir,
		obWriter:    obWriter,
		tradeWriter: tradeWriter,
	}, nil
}

// Start subscribes to the event bus and launches goroutines that consume
// order book and trade events, writing them to disk as JSONL records.
func (r *Recorder) Start(ctx context.Context) {
	obID, obCh := r.bus.OrderBooks.Subscribe(256)
	r.obSubID = obID

	tradeID, tradeCh := r.bus.Trades.Subscribe(1024)
	r.tradeSubID = tradeID

	r.wg.Add(2)

	go func() {
		defer r.wg.Done()
		for {
			select {
			case ob, ok := <-obCh:
				if !ok {
					return
				}
				r.recordOrderBook(ob)
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		defer r.wg.Done()
		for {
			select {
			case trade, ok := <-tradeCh:
				if !ok {
					return
				}
				r.recordTrade(trade)
			case <-ctx.Done():
				return
			}
		}
	}()

	r.logger.Info("recorder started",
		slog.String("dir", r.dir),
	)
}

// Stop unsubscribes from the event bus, waits for goroutines to finish,
// closes the writers, and logs final record counts.
func (r *Recorder) Stop() {
	r.bus.OrderBooks.Unsubscribe(r.obSubID)
	r.bus.Trades.Unsubscribe(r.tradeSubID)

	r.wg.Wait()

	if err := r.obWriter.Close(); err != nil {
		r.logger.Error("recorder: close ob writer", slog.String("error", err.Error()))
	}
	if err := r.tradeWriter.Close(); err != nil {
		r.logger.Error("recorder: close trade writer", slog.String("error", err.Error()))
	}

	r.logger.Info("recorder stopped",
		slog.Int64("orderbooks_recorded", r.obCount.Load()),
		slog.Int64("trades_recorded", r.tradeCount.Load()),
	)
}

// recordOrderBook converts an OrderBook into a compact OrderBookRecord and
// writes it to the order book writer.
func (r *Recorder) recordOrderBook(ob *models.OrderBook) {
	bids := ob.Bids()
	asks := ob.Asks()

	bidRecords := make([]LevelRecord, 0, len(bids))
	for i := range bids {
		if bids[i].Price != nil && bids[i].Size != nil {
			bidRecords = append(bidRecords, LevelRecord{
				Price: *bids[i].Price,
				Size:  *bids[i].Size,
			})
		}
	}

	askRecords := make([]LevelRecord, 0, len(asks))
	for i := range asks {
		if asks[i].Price != nil && asks[i].Size != nil {
			askRecords = append(askRecords, LevelRecord{
				Price: *asks[i].Price,
				Size:  *asks[i].Size,
			})
		}
	}

	var exchangeTS int64
	if ob.LastUpdated != nil {
		exchangeTS = ob.LastUpdated.UnixMilli()
	}

	rec := OrderBookRecord{
		Type:       "orderbook",
		Symbol:     ob.Symbol,
		Provider:   ob.ProviderName,
		Sequence:   ob.Sequence,
		ExchangeTS: exchangeTS,
		LocalTS:    time.Now().UnixMilli(),
		MidPrice:   ob.MidPrice(),
		Spread:     ob.Spread(),
		MicroPrice: ob.MicroPrice(),
		Bids:       bidRecords,
		Asks:       askRecords,
	}

	if err := r.obWriter.Write(rec); err != nil {
		r.logger.Error("recorder: write orderbook",
			slog.String("symbol", ob.Symbol),
			slog.String("error", err.Error()),
		)
		return
	}
	r.obCount.Add(1)
}

// recordTrade converts a Trade into a compact TradeRecord and writes it
// to the trade writer.
func (r *Recorder) recordTrade(t models.Trade) {
	rec := TradeRecord{
		Type:       "trade",
		Symbol:     t.Symbol,
		Provider:   t.ProviderName,
		Price:      t.Price.String(),
		Size:       t.Size.String(),
		ExchangeTS: t.Timestamp.UnixMilli(),
		LocalTS:    time.Now().UnixMilli(),
		IsBuy:      t.IsBuy,
		MidPrice:   t.MarketMidPrice,
	}

	if err := r.tradeWriter.Write(rec); err != nil {
		r.logger.Error("recorder: write trade",
			slog.String("symbol", t.Symbol),
			slog.String("error", err.Error()),
		)
		return
	}
	r.tradeCount.Add(1)
}
