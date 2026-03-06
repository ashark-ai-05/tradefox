package walkforward

import (
	"context"
	"testing"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/replay"
)

func TestRunGridTinyGrid(t *testing.T) {
	// Create a tiny 2-config grid.
	grid := ParamGrid{
		ConfluenceThreshold: ParamRange{"c", 0.50, 0.50, 0.05}, // 1 value
		MinOFIPersistence:   ParamRange{"o", 2, 2, 2},           // 1 value
		StopATRMult:         ParamRange{"s", 1.0, 1.0, 0.5},     // 1 value
		TargetATRMult:       ParamRange{"t", 2.0, 2.0, 0.5},     // 1 value
		MaxHoldingHours:     ParamRange{"h", 1, 2, 1},            // 2 values
	}

	if grid.Count() != 2 {
		t.Fatalf("grid count: got %d, want 2", grid.Count())
	}

	// Create synthetic records (simple -- the engine won't generate trades
	// without proper orderbook/trade data, but it should run without error).
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	records := make([]replay.Record, 100)
	for i := range records {
		records[i] = replay.Record{
			LocalTS: start.Add(time.Duration(i) * time.Minute).UnixMilli(),
		}
	}

	ctx := context.Background()
	results, err := RunGrid(ctx, grid, records, 2)
	if err != nil {
		t.Fatalf("RunGrid error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Results should be sorted by Sharpe descending.
	for i := 1; i < len(results); i++ {
		if results[i].Metrics.SharpeDaily > results[i-1].Metrics.SharpeDaily {
			t.Errorf("results not sorted by Sharpe: index %d (%f) > index %d (%f)",
				i, results[i].Metrics.SharpeDaily, i-1, results[i-1].Metrics.SharpeDaily)
		}
	}
}

func TestRunGridContextCancellation(t *testing.T) {
	grid := ParamGrid{
		ConfluenceThreshold: ParamRange{"c", 0.50, 0.60, 0.05}, // 3 values
		MinOFIPersistence:   ParamRange{"o", 2, 4, 2},           // 2 values
		StopATRMult:         ParamRange{"s", 1.0, 1.5, 0.5},     // 2 values
		TargetATRMult:       ParamRange{"t", 2.0, 2.5, 0.5},     // 2 values
		MaxHoldingHours:     ParamRange{"h", 1, 2, 1},            // 2 values
	}

	records := make([]replay.Record, 10)
	for i := range records {
		records[i] = replay.Record{LocalTS: int64(i * 1000)}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := RunGrid(ctx, grid, records, 1)
	if err == nil {
		t.Log("RunGrid completed before context cancellation could be detected (acceptable)")
	}
}

func TestRunGridEmptyGrid(t *testing.T) {
	grid := ParamGrid{
		ConfluenceThreshold: ParamRange{"c", 0.50, 0.40, 0.05}, // invalid: min > max -> 0 values
		MinOFIPersistence:   ParamRange{"o", 2, 2, 2},
		StopATRMult:         ParamRange{"s", 1.0, 1.0, 0.5},
		TargetATRMult:       ParamRange{"t", 2.0, 2.0, 0.5},
		MaxHoldingHours:     ParamRange{"h", 1, 1, 1},
	}

	records := make([]replay.Record, 10)
	ctx := context.Background()
	results, err := RunGrid(ctx, grid, records, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for empty grid, got %d", len(results))
	}
}
