package backtest

import (
	"testing"

	"github.com/ashark-ai-05/tradefox/internal/recorder"
	"github.com/ashark-ai-05/tradefox/internal/replay"
)

func TestSignalFeed_OBProducesEvent(t *testing.T) {
	feed := NewSignalFeed()
	rec := replay.Record{
		LocalTS: 1000, Type: "orderbook",
		OB: &recorder.OrderBookRecord{
			Symbol:     "SOLUSDT",
			MidPrice:   100.0,
			MicroPrice: 100.1,
			Bids:       []recorder.LevelRecord{{Price: 99.5, Size: 10}},
			Asks:       []recorder.LevelRecord{{Price: 100.5, Size: 8}},
			LocalTS:    1000,
		},
	}
	evt := feed.Process(rec)
	if evt == nil {
		t.Fatal("expected event")
	}
	if evt.Symbol != "SOLUSDT" {
		t.Error("wrong symbol")
	}
	if evt.MidPrice != 100.0 {
		t.Error("wrong mid")
	}
	if evt.BestBid != 99.5 {
		t.Error("wrong bid")
	}
	if evt.BestAsk != 100.5 {
		t.Error("wrong ask")
	}
}

func TestSignalFeed_TradeNoEvent(t *testing.T) {
	feed := NewSignalFeed()
	isBuy := true
	rec := replay.Record{
		LocalTS: 1000, Type: "trade",
		Trade: &recorder.TradeRecord{
			Symbol:     "SOLUSDT",
			Price:      "100",
			Size:       "1",
			IsBuy:      &isBuy,
			ExchangeTS: 1000,
		},
	}
	if feed.Process(rec) != nil {
		t.Error("trade should not produce event")
	}
}

func TestSignalFeed_NilRecord(t *testing.T) {
	feed := NewSignalFeed()
	rec := replay.Record{LocalTS: 1000, Type: "ohlcv"}
	if feed.Process(rec) != nil {
		t.Error("nil OB/Trade should not produce event")
	}
}

func TestSignalFeed_TradeAccumulatesState(t *testing.T) {
	feed := NewSignalFeed()
	isBuy := true

	// Feed a trade first
	tradeRec := replay.Record{
		LocalTS: 1000, Type: "trade",
		Trade: &recorder.TradeRecord{
			Symbol:     "SOLUSDT",
			Price:      "100.5",
			Size:       "5.0",
			IsBuy:      &isBuy,
			ExchangeTS: 1000,
		},
	}
	feed.Process(tradeRec)

	// Then feed an OB update
	obRec := replay.Record{
		LocalTS: 1001, Type: "orderbook",
		OB: &recorder.OrderBookRecord{
			Symbol:     "SOLUSDT",
			MidPrice:   100.0,
			MicroPrice: 100.1,
			Bids:       []recorder.LevelRecord{{Price: 99.5, Size: 10}},
			Asks:       []recorder.LevelRecord{{Price: 100.5, Size: 8}},
			LocalTS:    1001,
		},
	}
	evt := feed.Process(obRec)
	if evt == nil {
		t.Fatal("expected event after trade + OB")
	}
	if evt.Signals == nil {
		t.Fatal("expected signals to be computed")
	}
}

func TestSignalFeed_MultipleSymbols(t *testing.T) {
	feed := NewSignalFeed()

	rec1 := replay.Record{
		LocalTS: 1000, Type: "orderbook",
		OB: &recorder.OrderBookRecord{
			Symbol:     "SOLUSDT",
			MidPrice:   100.0,
			MicroPrice: 100.1,
			Bids:       []recorder.LevelRecord{{Price: 99.5, Size: 10}},
			Asks:       []recorder.LevelRecord{{Price: 100.5, Size: 8}},
			LocalTS:    1000,
		},
	}
	rec2 := replay.Record{
		LocalTS: 1001, Type: "orderbook",
		OB: &recorder.OrderBookRecord{
			Symbol:     "BTCUSDT",
			MidPrice:   50000.0,
			MicroPrice: 50001.0,
			Bids:       []recorder.LevelRecord{{Price: 49999, Size: 1}},
			Asks:       []recorder.LevelRecord{{Price: 50001, Size: 1}},
			LocalTS:    1001,
		},
	}

	evt1 := feed.Process(rec1)
	evt2 := feed.Process(rec2)

	if evt1 == nil || evt2 == nil {
		t.Fatal("expected events for both symbols")
	}
	if evt1.Symbol != "SOLUSDT" || evt2.Symbol != "BTCUSDT" {
		t.Error("symbols should be independent")
	}
}
