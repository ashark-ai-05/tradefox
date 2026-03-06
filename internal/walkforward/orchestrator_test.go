package walkforward

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/backtest"
	"github.com/ashark-ai-05/tradefox/internal/replay"
)

func TestRunWalkForwardSingleFold(t *testing.T) {
	// Create synthetic records spanning enough time for exactly 1 fold
	// with a tiny fold config (short durations).
	cfg := WalkForwardConfig{
		Grid: ParamGrid{
			ConfluenceThreshold: ParamRange{"c", 0.50, 0.50, 0.05}, // 1 value
			MinOFIPersistence:   ParamRange{"o", 2, 2, 2},           // 1 value
			StopATRMult:         ParamRange{"s", 1.0, 1.0, 0.5},     // 1 value
			TargetATRMult:       ParamRange{"t", 2.0, 2.0, 0.5},     // 1 value
			MaxHoldingHours:     ParamRange{"h", 1, 1, 1},            // 1 value
		},
		Folds: FoldConfig{
			TrainDuration: 2 * time.Hour,
			ValDuration:   1 * time.Hour,
			TestDuration:  1 * time.Hour,
			StepDuration:  1 * time.Hour,
		},
		Workers:       1,
		InitialEquity: 10000,
	}

	// Generate 5 hours of records (1 per minute = 300 records).
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	records := make([]replay.Record, 300)
	for i := range records {
		records[i] = replay.Record{
			LocalTS: start.Add(time.Duration(i) * time.Minute).UnixMilli(),
		}
	}

	ctx := context.Background()
	result, err := RunWalkForward(ctx, records, cfg)
	if err != nil {
		t.Fatalf("RunWalkForward error: %v", err)
	}

	if result == nil {
		t.Fatal("result is nil")
	}

	if len(result.Folds) == 0 {
		t.Fatal("expected at least 1 fold, got 0")
	}

	if result.Summary.NumFolds != len(result.Folds) {
		t.Errorf("NumFolds mismatch: summary=%d, actual=%d",
			result.Summary.NumFolds, len(result.Folds))
	}

	// Verify stability analysis is populated.
	if result.Stability.Verdict == "" {
		t.Error("stability verdict is empty")
	}

	t.Logf("Folds: %d, AvgTestSharpe: %.2f, Verdict: %s",
		result.Summary.NumFolds,
		result.Summary.AvgTestSharpe,
		result.Stability.Verdict,
	)
}

func TestRunWalkForwardNoFolds(t *testing.T) {
	cfg := DefaultWalkForwardConfig()

	// Too few records to produce any folds.
	records := []replay.Record{
		{LocalTS: time.Now().UnixMilli()},
	}

	ctx := context.Background()
	_, err := RunWalkForward(ctx, records, cfg)
	if err == nil {
		t.Error("expected error for insufficient data, got nil")
	}
}

func TestRunWalkForwardContextCanceled(t *testing.T) {
	cfg := WalkForwardConfig{
		Grid: ParamGrid{
			ConfluenceThreshold: ParamRange{"c", 0.50, 0.50, 0.05},
			MinOFIPersistence:   ParamRange{"o", 2, 2, 2},
			StopATRMult:         ParamRange{"s", 1.0, 1.0, 0.5},
			TargetATRMult:       ParamRange{"t", 2.0, 2.0, 0.5},
			MaxHoldingHours:     ParamRange{"h", 1, 1, 1},
		},
		Folds: FoldConfig{
			TrainDuration: 2 * time.Hour,
			ValDuration:   1 * time.Hour,
			TestDuration:  1 * time.Hour,
			StepDuration:  1 * time.Hour,
		},
		Workers:       1,
		InitialEquity: 10000,
	}

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	records := make([]replay.Record, 600) // 10 hours
	for i := range records {
		records[i] = replay.Record{
			LocalTS: start.Add(time.Duration(i) * time.Minute).UnixMilli(),
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := RunWalkForward(ctx, records, cfg)
	if err == nil {
		t.Error("expected error from canceled context, got nil")
	}
}

func TestComputeSummary(t *testing.T) {
	folds := []FoldResult{
		{
			TrainMetrics: backtest.BacktestMetrics{SharpeDaily: 2.0, WinRate: 60, TotalTrades: 50},
			TestMetrics:  backtest.BacktestMetrics{SharpeDaily: 1.5, WinRate: 55, TotalTrades: 30},
		},
		{
			TrainMetrics: backtest.BacktestMetrics{SharpeDaily: 2.5, WinRate: 65, TotalTrades: 60},
			TestMetrics:  backtest.BacktestMetrics{SharpeDaily: 2.0, WinRate: 60, TotalTrades: 35},
		},
	}

	s := computeSummary(folds)

	if s.NumFolds != 2 {
		t.Errorf("NumFolds: got %d, want 2", s.NumFolds)
	}

	wantAvgTestSharpe := (1.5 + 2.0) / 2.0
	if s.AvgTestSharpe != wantAvgTestSharpe {
		t.Errorf("AvgTestSharpe: got %.2f, want %.2f", s.AvgTestSharpe, wantAvgTestSharpe)
	}

	wantAvgTestWinRate := (55.0 + 60.0) / 2.0
	if s.AvgTestWinRate != wantAvgTestWinRate {
		t.Errorf("AvgTestWinRate: got %.2f, want %.2f", s.AvgTestWinRate, wantAvgTestWinRate)
	}

	if s.TotalTestTrades != 65 {
		t.Errorf("TotalTestTrades: got %d, want 65", s.TotalTestTrades)
	}

	wantAvgTrainSharpe := (2.0 + 2.5) / 2.0
	if s.AvgTrainSharpe != wantAvgTrainSharpe {
		t.Errorf("AvgTrainSharpe: got %.2f, want %.2f", s.AvgTrainSharpe, wantAvgTrainSharpe)
	}

	wantRatio := wantAvgTrainSharpe / wantAvgTestSharpe
	if s.TrainTestRatio != wantRatio {
		t.Errorf("TrainTestRatio: got %.2f, want %.2f", s.TrainTestRatio, wantRatio)
	}
}

func TestComputeSummaryEmpty(t *testing.T) {
	s := computeSummary(nil)
	if s.NumFolds != 0 {
		t.Errorf("NumFolds: got %d, want 0", s.NumFolds)
	}
}

func TestDefaultWalkForwardConfig(t *testing.T) {
	cfg := DefaultWalkForwardConfig()
	if cfg.Workers != 4 {
		t.Errorf("Workers: got %d, want 4", cfg.Workers)
	}
	if cfg.InitialEquity != 10000.0 {
		t.Errorf("InitialEquity: got %.1f, want 10000.0", cfg.InitialEquity)
	}
	if cfg.Grid.Count() == 0 {
		t.Error("Grid.Count() should not be 0")
	}
}

func TestRecommendedConfig(t *testing.T) {
	result := &WalkForwardResult{
		Folds: []FoldResult{
			makeFoldResult(0.50, 2, 1.0, 2.0, 1),
			makeFoldResult(0.60, 4, 1.5, 3.0, 2),
			makeFoldResult(0.70, 6, 2.0, 4.0, 4),
		},
	}

	cfg := result.RecommendedConfig()

	// Median of [0.50, 0.60, 0.70] = 0.60
	if cfg.Strategy.ConfluenceThreshold != 0.60 {
		t.Errorf("ConfluenceThreshold: got %.2f, want 0.60", cfg.Strategy.ConfluenceThreshold)
	}

	// Median of [2, 4, 6] = 4
	if cfg.Strategy.MinOFIPersistence != 4 {
		t.Errorf("MinOFIPersistence: got %d, want 4", cfg.Strategy.MinOFIPersistence)
	}

	// Median of [1.0, 1.5, 2.0] = 1.5
	if cfg.Position.StopATRMult != 1.5 {
		t.Errorf("StopATRMult: got %.1f, want 1.5", cfg.Position.StopATRMult)
	}

	// Median of [2.0, 3.0, 4.0] = 3.0
	if cfg.Position.TargetATRMult != 3.0 {
		t.Errorf("TargetATRMult: got %.1f, want 3.0", cfg.Position.TargetATRMult)
	}

	// Median of [1h, 2h, 4h] in ms = 2h in ms
	wantHoldMs := int64(2 * 60 * 60 * 1000)
	if cfg.Position.MaxHoldingMs != wantHoldMs {
		t.Errorf("MaxHoldingMs: got %d, want %d", cfg.Position.MaxHoldingMs, wantHoldMs)
	}
}

func TestRecommendedConfigEmpty(t *testing.T) {
	result := &WalkForwardResult{}
	cfg := result.RecommendedConfig()

	// Should return default config when no folds.
	def := backtest.DefaultEngineConfig()
	if cfg.Strategy.ConfluenceThreshold != def.Strategy.ConfluenceThreshold {
		t.Errorf("expected default confluence threshold")
	}
}

func TestWriteSummary(t *testing.T) {
	result := &WalkForwardResult{
		Folds: []FoldResult{
			{
				FoldIndex:  0,
				BestConfig: backtest.DefaultEngineConfig(),
				TrainMetrics: backtest.BacktestMetrics{
					SharpeDaily: 2.3, WinRate: 65.2, TotalTrades: 142,
				},
				ValMetrics: backtest.BacktestMetrics{
					SharpeDaily: 1.8, WinRate: 63.1, TotalTrades: 34,
				},
				TestMetrics: backtest.BacktestMetrics{
					SharpeDaily: 1.5, WinRate: 62.0, TotalTrades: 31,
				},
				TrainPeriod: "2025-01-01 to 2025-01-29",
				ValPeriod:   "2025-01-29 to 2025-02-05",
				TestPeriod:  "2025-02-05 to 2025-02-12",
			},
		},
		Summary: WFSummary{
			NumFolds:        1,
			AvgTestSharpe:   1.5,
			AvgTestWinRate:  62.0,
			TotalTestTrades: 31,
			AvgTrainSharpe:  2.3,
			TrainTestRatio:  1.53,
		},
		Stability: StabilityAnalysis{
			ParamStats: []ParamStability{
				{Name: "ConfluenceThreshold", Mean: 0.60, StdDev: 0.02, CV: 0.033},
			},
			Verdict: "stable",
			Flags:   []string{},
		},
	}

	var buf bytes.Buffer
	result.WriteSummary(&buf)
	output := buf.String()

	// Verify key sections are present.
	checks := []string{
		"WALK-FORWARD OPTIMIZATION RESULTS",
		"Fold 1",
		"Train ->",
		"Val   ->",
		"Test  ->",
		"Summary",
		"Stability",
		"stable",
	}
	for _, check := range checks {
		if !bytes.Contains([]byte(output), []byte(check)) {
			t.Errorf("output missing %q", check)
		}
	}
}
