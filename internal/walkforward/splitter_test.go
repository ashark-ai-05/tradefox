package walkforward

import (
	"testing"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/replay"
)

// makeRecords creates synthetic records spanning a duration with a given interval.
func makeRecords(start time.Time, duration, interval time.Duration) []replay.Record {
	var records []replay.Record
	for t := start; t.Before(start.Add(duration)); t = t.Add(interval) {
		records = append(records, replay.Record{LocalTS: t.UnixMilli()})
	}
	return records
}

func TestSplitFoldsBasic(t *testing.T) {
	// 8 weeks of data, 1 record per hour.
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	records := makeRecords(start, 8*7*24*time.Hour, time.Hour)

	cfg := DefaultFoldConfig() // 4w train, 1w val, 1w test, 1w step
	folds := SplitFolds(records, cfg)

	// 8 weeks = 56 days. Each fold needs 6 weeks. Step 1 week.
	// Fold 0: week 0-3 train, week 4 val, week 5 test (end at week 6)
	// Fold 1: week 1-4 train, week 5 val, week 6 test (end at week 7)
	// Fold 2: week 2-5 train, week 6 val, week 7 test (end at week 8) -- needs data until end of week 8
	// But we have exactly 8 weeks of data, so the last record is at week 8 - 1hour.
	// Fold 2 test end = week 8 exactly. The window end is exactly at the end of data.
	// Since we use [start, end) and data goes up to week8 - 1h, fold 2's test period
	// extends to week 8, but the last record at week 7 + 6d + 23h is < week 8, so fold 2 should work.
	if len(folds) < 2 {
		t.Fatalf("expected at least 2 folds, got %d", len(folds))
	}

	t.Logf("got %d folds from 8 weeks of data", len(folds))
}

func TestSplitFoldsNonOverlapping(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	records := makeRecords(start, 10*7*24*time.Hour, time.Hour)

	cfg := DefaultFoldConfig()
	folds := SplitFolds(records, cfg)

	for _, f := range folds {
		// Within each fold, segments must not overlap.
		if !f.TrainEnd.Equal(f.ValStart) {
			t.Errorf("fold %d: train end %v != val start %v", f.Index, f.TrainEnd, f.ValStart)
		}
		if !f.ValEnd.Equal(f.TestStart) {
			t.Errorf("fold %d: val end %v != test start %v", f.Index, f.ValEnd, f.TestStart)
		}
		if f.TrainStart.After(f.TrainEnd) {
			t.Errorf("fold %d: train start after end", f.Index)
		}
		if f.TestStart.After(f.TestEnd) {
			t.Errorf("fold %d: test start after end", f.Index)
		}

		// Verify records are within expected time ranges.
		for _, r := range f.Train {
			ts := time.UnixMilli(r.LocalTS)
			if ts.Before(f.TrainStart) || !ts.Before(f.TrainEnd) {
				t.Errorf("fold %d: train record at %v outside [%v, %v)", f.Index, ts, f.TrainStart, f.TrainEnd)
			}
		}
		for _, r := range f.Val {
			ts := time.UnixMilli(r.LocalTS)
			if ts.Before(f.ValStart) || !ts.Before(f.ValEnd) {
				t.Errorf("fold %d: val record at %v outside [%v, %v)", f.Index, ts, f.ValStart, f.ValEnd)
			}
		}
		for _, r := range f.Test {
			ts := time.UnixMilli(r.LocalTS)
			if ts.Before(f.TestStart) || !ts.Before(f.TestEnd) {
				t.Errorf("fold %d: test record at %v outside [%v, %v)", f.Index, ts, f.TestStart, f.TestEnd)
			}
		}
	}
}

func TestSplitFoldsEmpty(t *testing.T) {
	folds := SplitFolds(nil, DefaultFoldConfig())
	if len(folds) != 0 {
		t.Fatalf("expected 0 folds for nil records, got %d", len(folds))
	}
}

func TestSplitFoldsTooShort(t *testing.T) {
	// Data shorter than one full fold.
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	records := makeRecords(start, 3*7*24*time.Hour, time.Hour) // 3 weeks < 6 weeks needed

	cfg := DefaultFoldConfig()
	folds := SplitFolds(records, cfg)

	if len(folds) != 0 {
		t.Fatalf("expected 0 folds for too-short data, got %d", len(folds))
	}
}

func TestSplitFoldsSubSlices(t *testing.T) {
	// Verify that returned slices share underlying array (sub-slices, no copy).
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	records := makeRecords(start, 8*7*24*time.Hour, time.Hour)

	cfg := DefaultFoldConfig()
	folds := SplitFolds(records, cfg)

	if len(folds) == 0 {
		t.Fatal("expected at least one fold")
	}

	// The first train record should point into the original records slice.
	f := folds[0]
	if len(f.Train) == 0 {
		t.Fatal("fold 0 train is empty")
	}
	// Verify it's a sub-slice by checking pointer identity.
	trainFirst := &f.Train[0]
	found := false
	for i := range records {
		if &records[i] == trainFirst {
			found = true
			break
		}
	}
	if !found {
		t.Error("train slice is not a sub-slice of original records")
	}
}

func TestSplitFoldsBoundaries(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	records := makeRecords(start, 8*7*24*time.Hour, time.Hour)

	cfg := DefaultFoldConfig()
	folds := SplitFolds(records, cfg)

	if len(folds) == 0 {
		t.Fatal("expected at least one fold")
	}

	// First fold should start at the beginning of data.
	f0 := folds[0]
	if !f0.TrainStart.Equal(start) {
		t.Errorf("first fold train start: got %v, want %v", f0.TrainStart, start)
	}

	// Train duration should be 4 weeks.
	trainDur := f0.TrainEnd.Sub(f0.TrainStart)
	if trainDur != 4*7*24*time.Hour {
		t.Errorf("train duration: got %v, want %v", trainDur, 4*7*24*time.Hour)
	}

	// Val duration should be 1 week.
	valDur := f0.ValEnd.Sub(f0.ValStart)
	if valDur != 7*24*time.Hour {
		t.Errorf("val duration: got %v, want %v", valDur, 7*24*time.Hour)
	}

	// Test duration should be 1 week.
	testDur := f0.TestEnd.Sub(f0.TestStart)
	if testDur != 7*24*time.Hour {
		t.Errorf("test duration: got %v, want %v", testDur, 7*24*time.Hour)
	}

	// Second fold should be shifted by 1 week.
	if len(folds) >= 2 {
		f1 := folds[1]
		step := f1.TrainStart.Sub(f0.TrainStart)
		if step != 7*24*time.Hour {
			t.Errorf("step between folds: got %v, want %v", step, 7*24*time.Hour)
		}
	}
}
