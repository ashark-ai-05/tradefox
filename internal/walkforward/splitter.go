package walkforward

import (
	"sort"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/replay"
)

// FoldConfig defines the durations for train/val/test windows and the step size.
type FoldConfig struct {
	TrainDuration time.Duration
	ValDuration   time.Duration
	TestDuration  time.Duration
	StepDuration  time.Duration
}

// DefaultFoldConfig returns the default fold configuration.
func DefaultFoldConfig() FoldConfig {
	return FoldConfig{
		TrainDuration: 4 * 7 * 24 * time.Hour, // 4 weeks
		ValDuration:   7 * 24 * time.Hour,      // 1 week
		TestDuration:  7 * 24 * time.Hour,      // 1 week
		StepDuration:  7 * 24 * time.Hour,      // 1 week
	}
}

// Fold represents a single train/val/test split of records.
type Fold struct {
	Index      int
	Train      []replay.Record
	Val        []replay.Record
	Test       []replay.Record
	TrainStart time.Time
	TrainEnd   time.Time
	ValStart   time.Time
	ValEnd     time.Time
	TestStart  time.Time
	TestEnd    time.Time
}

// SplitFolds creates rolling window folds. Uses sort.Search on LocalTS
// for efficient boundary finding. Returns sub-slices (no copying).
func SplitFolds(records []replay.Record, cfg FoldConfig) []Fold {
	if len(records) == 0 {
		return nil
	}

	totalWindow := cfg.TrainDuration + cfg.ValDuration + cfg.TestDuration
	startTS := records[0].LocalTS
	endTS := records[len(records)-1].LocalTS

	startTime := time.UnixMilli(startTS)
	endTime := time.UnixMilli(endTS)

	var folds []Fold
	foldIdx := 0

	for windowStart := startTime; ; windowStart = windowStart.Add(cfg.StepDuration) {
		windowEnd := windowStart.Add(totalWindow)
		if windowEnd.After(endTime) {
			break
		}

		trainStart := windowStart
		trainEnd := trainStart.Add(cfg.TrainDuration)
		valStart := trainEnd
		valEnd := valStart.Add(cfg.ValDuration)
		testStart := valEnd
		testEnd := testStart.Add(cfg.TestDuration)

		trainSlice := sliceByTime(records, trainStart, trainEnd)
		valSlice := sliceByTime(records, valStart, valEnd)
		testSlice := sliceByTime(records, testStart, testEnd)

		// Skip folds where any segment is empty.
		if len(trainSlice) == 0 || len(valSlice) == 0 || len(testSlice) == 0 {
			continue
		}

		folds = append(folds, Fold{
			Index:      foldIdx,
			Train:      trainSlice,
			Val:        valSlice,
			Test:       testSlice,
			TrainStart: trainStart,
			TrainEnd:   trainEnd,
			ValStart:   valStart,
			ValEnd:     valEnd,
			TestStart:  testStart,
			TestEnd:    testEnd,
		})
		foldIdx++
	}

	return folds
}

// sliceByTime returns a sub-slice of records with LocalTS in [start, end).
// Uses sort.Search for efficient binary search on the sorted records.
func sliceByTime(records []replay.Record, start, end time.Time) []replay.Record {
	startMs := start.UnixMilli()
	endMs := end.UnixMilli()

	lo := sort.Search(len(records), func(i int) bool {
		return records[i].LocalTS >= startMs
	})
	hi := sort.Search(len(records), func(i int) bool {
		return records[i].LocalTS >= endMs
	})

	if lo >= hi {
		return nil
	}
	return records[lo:hi]
}
