package backtest

import (
	"log/slog"
	"os"
	"sort"
	"testing"

	"github.com/ashark-ai-05/tradefox/internal/recorder"
	"github.com/ashark-ai-05/tradefox/internal/replay"
)

func TestEngine_BasicRun(t *testing.T) {
	cfg := DefaultEngineConfig()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	engine := NewEngine(cfg, logger)

	var records []replay.Record
	for i := 0; i < 20; i++ {
		records = append(records, replay.Record{
			LocalTS: int64(i) * 60000, Type: "ohlcv",
			Kiy: &recorder.KiyotakaRecord{Type: "ohlcv", Symbol: "SOLUSDT", High: 100 + float64(i%5), Low: 95 + float64(i%5), Close: 98 + float64(i%5)},
		})
	}
	isBuy := true
	for i := 0; i < 100; i++ {
		records = append(records, replay.Record{
			LocalTS: int64(20+i) * 60000, Type: "trade",
			Trade: &recorder.TradeRecord{Symbol: "SOLUSDT", Price: "100.0", Size: "1.0", IsBuy: &isBuy, ExchangeTS: int64(20+i) * 60000},
		})
	}
	for i := 0; i < 50; i++ {
		records = append(records, replay.Record{
			LocalTS: int64(120+i) * 60000, Type: "orderbook",
			OB: &recorder.OrderBookRecord{
				Symbol: "SOLUSDT", MidPrice: 100.0 + float64(i%10)*0.1, MicroPrice: 100.05 + float64(i%10)*0.1, Spread: 0.10,
				Bids: []recorder.LevelRecord{{Price: 99.95, Size: 10}}, Asks: []recorder.LevelRecord{{Price: 100.05, Size: 8}}, LocalTS: int64(120+i) * 60000,
			},
		})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].LocalTS < records[j].LocalTS })

	result, err := engine.Run(records)
	if err != nil {
		t.Fatal(err)
	}
	if result.DataStats.TotalRecords != int64(len(records)) {
		t.Error("wrong total")
	}
	if result.DataStats.OBRecords != 50 {
		t.Errorf("expected 50 OB, got %d", result.DataStats.OBRecords)
	}
}

func TestEngine_EmptyData(t *testing.T) {
	cfg := DefaultEngineConfig()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	result, err := NewEngine(cfg, logger).Run(nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TotalTrades != 0 {
		t.Error("expected 0 trades")
	}
}
