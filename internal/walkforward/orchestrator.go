package walkforward

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ashark-ai-05/tradefox/internal/replay"
)

// WalkForwardConfig holds the full walk-forward optimization config.
type WalkForwardConfig struct {
	Grid          ParamGrid
	Folds         FoldConfig
	Workers       int
	InitialEquity float64
}

// DefaultWalkForwardConfig returns the default walk-forward configuration.
func DefaultWalkForwardConfig() WalkForwardConfig {
	return WalkForwardConfig{
		Grid:          DefaultParamGrid(),
		Folds:         DefaultFoldConfig(),
		Workers:       4,
		InitialEquity: 10000.0,
	}
}

// WalkForwardResult holds the complete walk-forward optimization result.
type WalkForwardResult struct {
	Folds     []FoldResult      `json:"folds"`
	Summary   WFSummary         `json:"summary"`
	Stability StabilityAnalysis `json:"stability"`
}

// WFSummary aggregates test performance across all folds.
type WFSummary struct {
	NumFolds        int     `json:"numFolds"`
	AvgTestSharpe   float64 `json:"avgTestSharpe"`
	AvgTestWinRate  float64 `json:"avgTestWinRate"`
	TotalTestTrades int     `json:"totalTestTrades"`
	AvgTrainSharpe  float64 `json:"avgTrainSharpe"`
	TrainTestRatio  float64 `json:"trainTestRatio"` // avg(train Sharpe) / avg(test Sharpe)
}

// RunWalkForward runs the complete walk-forward optimization.
// Iterates over each fold sequentially (each fold runs grid search in parallel internally).
func RunWalkForward(ctx context.Context, records []replay.Record, cfg WalkForwardConfig) (*WalkForwardResult, error) {
	folds := SplitFolds(records, cfg.Folds)
	if len(folds) == 0 {
		return nil, fmt.Errorf("walkforward: no folds generated from %d records", len(records))
	}

	slog.Info("walk-forward optimization starting",
		"folds", len(folds),
		"gridSize", cfg.Grid.Count(),
		"workers", cfg.Workers,
	)

	var results []FoldResult
	for i, fold := range folds {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		slog.Info("evaluating fold",
			"fold", i+1,
			"of", len(folds),
			"trainRecords", len(fold.Train),
			"valRecords", len(fold.Val),
			"testRecords", len(fold.Test),
		)

		foldResult, err := EvaluateFold(ctx, fold, cfg.Grid, cfg.Workers)
		if err != nil {
			return nil, fmt.Errorf("fold %d: %w", i+1, err)
		}
		foldResult.FoldIndex = i
		results = append(results, *foldResult)

		slog.Info("fold complete",
			"fold", i+1,
			"trainSharpe", foldResult.TrainMetrics.SharpeDaily,
			"testSharpe", foldResult.TestMetrics.SharpeDaily,
		)
	}

	summary := computeSummary(results)
	stability := analyzeStability(results)

	return &WalkForwardResult{
		Folds:     results,
		Summary:   summary,
		Stability: stability,
	}, nil
}

// computeSummary aggregates test performance across folds.
func computeSummary(folds []FoldResult) WFSummary {
	if len(folds) == 0 {
		return WFSummary{}
	}

	s := WFSummary{
		NumFolds: len(folds),
	}

	var totalTestSharpe, totalTestWinRate, totalTrainSharpe float64
	for _, f := range folds {
		totalTestSharpe += f.TestMetrics.SharpeDaily
		totalTestWinRate += f.TestMetrics.WinRate
		s.TotalTestTrades += f.TestMetrics.TotalTrades
		totalTrainSharpe += f.TrainMetrics.SharpeDaily
	}

	n := float64(len(folds))
	s.AvgTestSharpe = totalTestSharpe / n
	s.AvgTestWinRate = totalTestWinRate / n
	s.AvgTrainSharpe = totalTrainSharpe / n

	if s.AvgTestSharpe != 0 {
		s.TrainTestRatio = s.AvgTrainSharpe / s.AvgTestSharpe
	}

	return s
}
