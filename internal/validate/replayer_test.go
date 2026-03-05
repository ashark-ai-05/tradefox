package validate

import (
	"testing"

	"github.com/ashark-ai-05/tradefox/internal/recorder"
	"github.com/ashark-ai-05/tradefox/internal/replay"
)

func TestReplayer_ProcessOB(t *testing.T) {
	r := NewReplayer()

	bids := []recorder.LevelRecord{{Price: 100.0, Size: 10.0}, {Price: 99.5, Size: 15.0}}
	asks := []recorder.LevelRecord{{Price: 100.5, Size: 8.0}, {Price: 101.0, Size: 12.0}}

	rec := replay.Record{
		LocalTS: 1700000000000,
		Type:    "orderbook",
		OB: &recorder.OrderBookRecord{
			Symbol:     "SOLUSDT",
			MidPrice:   100.25,
			MicroPrice: 100.28,
			Bids:       bids,
			Asks:       asks,
			LocalTS:    1700000000000,
		},
	}

	snap := r.Process(rec)
	if snap == nil {
		t.Fatal("expected snapshot from OB record")
	}
	if snap.Symbol != "SOLUSDT" {
		t.Errorf("expected SOLUSDT, got %s", snap.Symbol)
	}
	if snap.MidPrice != 100.25 {
		t.Errorf("expected mid 100.25, got %f", snap.MidPrice)
	}
	if snap.Signals == nil {
		t.Fatal("expected signals to be computed")
	}
}

func TestReplayer_TradeDoesNotProduceSnapshot(t *testing.T) {
	r := NewReplayer()
	isBuy := true
	rec := replay.Record{
		LocalTS: 1700000000000,
		Type:    "trade",
		Trade: &recorder.TradeRecord{
			Symbol:     "SOLUSDT",
			Price:      "100.50",
			Size:       "1.0",
			IsBuy:      &isBuy,
			ExchangeTS: 1700000000000,
		},
	}
	snap := r.Process(rec)
	if snap != nil {
		t.Error("trade should not produce a snapshot")
	}
}

func TestReplayer_MultipleOBUpdates(t *testing.T) {
	r := NewReplayer()

	// First OB
	snap1 := r.Process(replay.Record{
		LocalTS: 1000,
		Type:    "orderbook",
		OB: &recorder.OrderBookRecord{
			Symbol:     "BTCUSDT",
			MidPrice:   50000.0,
			MicroPrice: 50001.0,
			Bids:       []recorder.LevelRecord{{Price: 49999, Size: 1}},
			Asks:       []recorder.LevelRecord{{Price: 50001, Size: 1}},
			LocalTS:    1000,
		},
	})
	if snap1 == nil {
		t.Fatal("expected snapshot from first OB")
	}

	// Second OB
	snap2 := r.Process(replay.Record{
		LocalTS: 2000,
		Type:    "orderbook",
		OB: &recorder.OrderBookRecord{
			Symbol:     "BTCUSDT",
			MidPrice:   50010.0,
			MicroPrice: 50011.0,
			Bids:       []recorder.LevelRecord{{Price: 50009, Size: 2}},
			Asks:       []recorder.LevelRecord{{Price: 50011, Size: 1}},
			LocalTS:    2000,
		},
	})
	if snap2 == nil {
		t.Fatal("expected snapshot from second OB")
	}
	if snap2.MidPrice != 50010.0 {
		t.Errorf("expected mid 50010.0, got %f", snap2.MidPrice)
	}
}

func TestReplayer_TradesThenOB(t *testing.T) {
	r := NewReplayer()

	// Feed some trades first
	isBuy := true
	for i := 0; i < 5; i++ {
		r.Process(replay.Record{
			LocalTS: int64(1000 + i*100),
			Type:    "trade",
			Trade: &recorder.TradeRecord{
				Symbol:     "SOLUSDT",
				Price:      "100.0",
				Size:       "5.0",
				IsBuy:      &isBuy,
				ExchangeTS: int64(1000 + i*100),
			},
		})
	}

	// Then an OB update should include trade state
	snap := r.Process(replay.Record{
		LocalTS: 2000,
		Type:    "orderbook",
		OB: &recorder.OrderBookRecord{
			Symbol:     "SOLUSDT",
			MidPrice:   100.25,
			MicroPrice: 100.28,
			Bids:       []recorder.LevelRecord{{Price: 100.0, Size: 10.0}},
			Asks:       []recorder.LevelRecord{{Price: 100.5, Size: 8.0}},
			LocalTS:    2000,
		},
	})
	if snap == nil {
		t.Fatal("expected snapshot")
	}
	if snap.Signals == nil {
		t.Fatal("expected signals to be computed")
	}
}

func TestReplayer_KiyotakaIgnored(t *testing.T) {
	r := NewReplayer()
	snap := r.Process(replay.Record{
		LocalTS: 1000,
		Type:    "oi",
		Kiy: &recorder.KiyotakaRecord{
			Type:   "oi",
			Symbol: "SOLUSDT",
			Value:  12345.0,
		},
	})
	if snap != nil {
		t.Error("kiyotaka record should not produce a snapshot")
	}
}
