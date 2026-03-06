package walkforward

import (
	"context"
	"log/slog"
	"sort"
	"sync"

	"github.com/ashark-ai-05/tradefox/internal/backtest"
	"github.com/ashark-ai-05/tradefox/internal/replay"
)

// GridResult holds the config and resulting metrics from a single backtest run.
type GridResult struct {
	Config  backtest.EngineConfig    `json:"config"`
	Metrics backtest.BacktestMetrics `json:"metrics"`
}

// RunGrid runs the engine for each config in the grid.
// Uses a semaphore-based worker pool for parallelism.
// Returns results sorted by Sharpe descending.
func RunGrid(ctx context.Context, grid ParamGrid, records []replay.Record, workers int) ([]GridResult, error) {
	configs := grid.Enumerate()
	if len(configs) == 0 {
		return nil, nil
	}
	if workers <= 0 {
		workers = 1
	}

	results := make([]GridResult, len(configs))
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for i, cfg := range configs {
		// Check context before launching.
		if ctx.Err() != nil {
			mu.Lock()
			if firstErr == nil {
				firstErr = ctx.Err()
			}
			mu.Unlock()
			break
		}

		wg.Add(1)
		sem <- struct{}{} // acquire semaphore

		go func(idx int, c backtest.EngineConfig) {
			defer wg.Done()
			defer func() { <-sem }() // release semaphore

			// Check context inside goroutine.
			select {
			case <-ctx.Done():
				mu.Lock()
				if firstErr == nil {
					firstErr = ctx.Err()
				}
				mu.Unlock()
				return
			default:
			}

			engine := backtest.NewEngine(c, slog.Default())
			result, err := engine.Run(records)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}

			results[idx] = GridResult{
				Config:  c,
				Metrics: result.Metrics,
			}
		}(i, cfg)
	}

	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	// Sort by Sharpe descending.
	sort.Slice(results, func(i, j int) bool {
		return results[i].Metrics.SharpeDaily > results[j].Metrics.SharpeDaily
	})

	return results, nil
}
