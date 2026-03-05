package validate

import (
	"math"
	"testing"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/signals"
)

func TestComputeForwardReturns(t *testing.T) {
	snapshots := []SignalSnapshot{
		{Timestamp: 1000, MidPrice: 100.0, Signals: &signals.SignalSet{}},
		{Timestamp: 2000, MidPrice: 101.0, Signals: &signals.SignalSet{}}, // +1s
		{Timestamp: 6000, MidPrice: 102.0, Signals: &signals.SignalSet{}}, // +5s from first
	}

	horizons := []time.Duration{1 * time.Second, 5 * time.Second}
	rows := ComputeForwardReturns(snapshots, horizons)

	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// First snapshot at t=1000: 1s horizon -> snapshot at t=2000 (price 101)
	ret1s := rows[0].Returns[1*time.Second]
	expected := math.Log(101.0 / 100.0)
	if math.Abs(ret1s-expected) > 0.0001 {
		t.Errorf("expected 1s return ~ %f, got %f", expected, ret1s)
	}

	// First snapshot at t=1000: 5s horizon -> snapshot at t=6000 (price 102)
	ret5s := rows[0].Returns[5*time.Second]
	expected = math.Log(102.0 / 100.0)
	if math.Abs(ret5s-expected) > 0.0001 {
		t.Errorf("expected 5s return ~ %f, got %f", expected, ret5s)
	}
}

func TestComputeForwardReturns_NaN(t *testing.T) {
	// Last snapshot should have NaN for future horizons
	snapshots := []SignalSnapshot{
		{Timestamp: 1000, MidPrice: 100.0, Signals: &signals.SignalSet{}},
	}

	horizons := []time.Duration{1 * time.Second}
	rows := ComputeForwardReturns(snapshots, horizons)

	ret := rows[0].Returns[1*time.Second]
	if !math.IsNaN(ret) {
		t.Errorf("expected NaN for missing future data, got %f", ret)
	}
}

func TestComputeForwardReturns_ZeroPrice(t *testing.T) {
	snapshots := []SignalSnapshot{
		{Timestamp: 1000, MidPrice: 0.0, Signals: &signals.SignalSet{}},
		{Timestamp: 2000, MidPrice: 100.0, Signals: &signals.SignalSet{}},
	}

	horizons := []time.Duration{1 * time.Second}
	rows := ComputeForwardReturns(snapshots, horizons)

	ret := rows[0].Returns[1*time.Second]
	if !math.IsNaN(ret) {
		t.Errorf("expected NaN for zero price, got %f", ret)
	}
}

func TestComputeForwardReturns_PreservesFields(t *testing.T) {
	ss := &signals.SignalSet{OFI: signals.OFISignal{Value: 0.5}}
	snapshots := []SignalSnapshot{
		{Timestamp: 1000, Symbol: "SOLUSDT", MidPrice: 100.0, Signals: ss},
	}

	horizons := []time.Duration{1 * time.Second}
	rows := ComputeForwardReturns(snapshots, horizons)

	if rows[0].Symbol != "SOLUSDT" {
		t.Errorf("expected SOLUSDT, got %s", rows[0].Symbol)
	}
	if rows[0].MidPrice != 100.0 {
		t.Errorf("expected 100.0, got %f", rows[0].MidPrice)
	}
	if rows[0].Signals != ss {
		t.Error("expected same signal set pointer")
	}
}
