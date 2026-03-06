package walkforward

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ashark-ai-05/tradefox/internal/backtest"
)

// FoldResult holds the evaluation results for a single fold.
type FoldResult struct {
	FoldIndex        int                      `json:"foldIndex"`
	BestConfig       backtest.EngineConfig    `json:"bestConfig"`
	TrainMetrics     backtest.BacktestMetrics `json:"trainMetrics"`
	ValMetrics       backtest.BacktestMetrics `json:"valMetrics"`
	TestMetrics      backtest.BacktestMetrics `json:"testMetrics"`
	ConfigsEvaluated int                      `json:"configsEvaluated"`
	TrainPeriod      string                   `json:"trainPeriod"`
	ValPeriod        string                   `json:"valPeriod"`
	TestPeriod       string                   `json:"testPeriod"`
}

// SelectBest picks the best config from training results.
// Filters out overfit candidates: Sharpe > 4 OR WinRate > 80% OR TotalTrades < 30.
// From remaining, picks highest Sharpe. Returns the result and its index.
func SelectBest(trainResults []GridResult) (GridResult, int) {
	bestIdx := -1
	var best GridResult

	for i, r := range trainResults {
		m := r.Metrics
		// Filter out overfit candidates.
		if m.SharpeDaily > 4 {
			continue
		}
		if m.WinRate > 80 {
			continue
		}
		if m.TotalTrades < 30 {
			continue
		}

		if bestIdx == -1 || m.SharpeDaily > best.Metrics.SharpeDaily {
			best = r
			bestIdx = i
		}
	}

	// If all configs are filtered out, fall back to highest Sharpe overall.
	if bestIdx == -1 && len(trainResults) > 0 {
		bestIdx = 0
		best = trainResults[0]
		for i, r := range trainResults[1:] {
			if r.Metrics.SharpeDaily > best.Metrics.SharpeDaily {
				best = r
				bestIdx = i + 1
			}
		}
	}

	return best, bestIdx
}

// EvaluateFold runs grid search on train, selects best, validates on val, evaluates on test.
func EvaluateFold(ctx context.Context, fold Fold, grid ParamGrid, workers int) (*FoldResult, error) {
	// Step 1: Run grid search on training data.
	trainResults, err := RunGrid(ctx, grid, fold.Train, workers)
	if err != nil {
		return nil, fmt.Errorf("fold %d train: %w", fold.Index, err)
	}

	if len(trainResults) == 0 {
		return nil, fmt.Errorf("fold %d: no training results", fold.Index)
	}

	// Step 2: Select best config from training.
	best, _ := SelectBest(trainResults)

	// Step 3: Run best config on validation data.
	valEngine := backtest.NewEngine(best.Config, slog.Default())
	valResult, err := valEngine.Run(fold.Val)
	if err != nil {
		return nil, fmt.Errorf("fold %d val: %w", fold.Index, err)
	}

	// Step 4: Run best config on test data.
	testEngine := backtest.NewEngine(best.Config, slog.Default())
	testResult, err := testEngine.Run(fold.Test)
	if err != nil {
		return nil, fmt.Errorf("fold %d test: %w", fold.Index, err)
	}

	const timeFmt = "2006-01-02"
	return &FoldResult{
		FoldIndex:        fold.Index,
		BestConfig:       best.Config,
		TrainMetrics:     best.Metrics,
		ValMetrics:       valResult.Metrics,
		TestMetrics:      testResult.Metrics,
		ConfigsEvaluated: len(trainResults),
		TrainPeriod:      fold.TrainStart.Format(timeFmt) + " to " + fold.TrainEnd.Format(timeFmt),
		ValPeriod:        fold.ValStart.Format(timeFmt) + " to " + fold.ValEnd.Format(timeFmt),
		TestPeriod:       fold.TestStart.Format(timeFmt) + " to " + fold.TestEnd.Format(timeFmt),
	}, nil
}
