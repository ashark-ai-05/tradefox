package validate

import (
	"math"
	"sort"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/signals"
)

// DefaultHorizons for forward return computation.
var DefaultHorizons = []time.Duration{
	1 * time.Second,
	5 * time.Second,
	10 * time.Second,
	30 * time.Second,
	60 * time.Second,
	2 * time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	60 * time.Minute,
}

// ReturnRow holds a snapshot with its forward returns at multiple horizons.
type ReturnRow struct {
	Timestamp int64
	Symbol    string
	MidPrice  float64
	Signals   *signals.SignalSet
	Returns   map[time.Duration]float64 // horizon -> log return (NaN if not available)
}

// ComputeForwardReturns computes log returns at each horizon for each snapshot.
// Uses binary search to find the closest future snapshot.
func ComputeForwardReturns(snapshots []SignalSnapshot, horizons []time.Duration) []ReturnRow {
	rows := make([]ReturnRow, len(snapshots))
	timestamps := make([]int64, len(snapshots))
	for i, s := range snapshots {
		timestamps[i] = s.Timestamp
	}

	for i, snap := range snapshots {
		returns := make(map[time.Duration]float64, len(horizons))
		for _, h := range horizons {
			targetTS := snap.Timestamp + h.Milliseconds()
			// Binary search for closest snapshot at or after targetTS
			idx := sort.Search(len(timestamps), func(j int) bool {
				return timestamps[j] >= targetTS
			})

			if idx < len(snapshots) {
				gap := time.Duration(snapshots[idx].Timestamp-targetTS) * time.Millisecond
				if gap <= 2*h || gap <= 5*time.Second { // tolerance
					futurePrice := snapshots[idx].MidPrice
					if snap.MidPrice > 0 && futurePrice > 0 {
						returns[h] = math.Log(futurePrice / snap.MidPrice)
					} else {
						returns[h] = math.NaN()
					}
				} else {
					returns[h] = math.NaN()
				}
			} else {
				returns[h] = math.NaN()
			}
		}
		rows[i] = ReturnRow{
			Timestamp: snap.Timestamp,
			Symbol:    snap.Symbol,
			MidPrice:  snap.MidPrice,
			Signals:   snap.Signals,
			Returns:   returns,
		}
	}
	return rows
}
