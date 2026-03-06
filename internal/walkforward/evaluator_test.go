package walkforward

import (
	"context"
	"testing"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/backtest"
	"github.com/ashark-ai-05/tradefox/internal/replay"
)

func TestSelectBestFiltersOverfit(t *testing.T) {
	results := []GridResult{
		{
			Config: backtest.DefaultEngineConfig(),
			Metrics: backtest.BacktestMetrics{
				SharpeDaily: 5.0, // overfit: Sharpe > 4
				WinRate:     60,
				TotalTrades: 50,
			},
		},
		{
			Config: backtest.DefaultEngineConfig(),
			Metrics: backtest.BacktestMetrics{
				SharpeDaily: 2.0, // good: passes all filters
				WinRate:     55,
				TotalTrades: 40,
			},
		},
		{
			Config: backtest.DefaultEngineConfig(),
			Metrics: backtest.BacktestMetrics{
				SharpeDaily: 3.0, // overfit: WinRate > 80%
				WinRate:     85,
				TotalTrades: 50,
			},
		},
		{
			Config: backtest.DefaultEngineConfig(),
			Metrics: backtest.BacktestMetrics{
				SharpeDaily: 3.5, // overfit: TotalTrades < 30
				WinRate:     60,
				TotalTrades: 10,
			},
		},
	}

	best, idx := SelectBest(results)

	// Should select index 1 (Sharpe 2.0) because all others are filtered.
	if best.Metrics.SharpeDaily != 2.0 {
		t.Errorf("expected Sharpe 2.0, got %f (index %d)", best.Metrics.SharpeDaily, idx)
	}
}

func TestSelectBestPicksHighestSharpe(t *testing.T) {
	results := []GridResult{
		{
			Config: backtest.DefaultEngineConfig(),
			Metrics: backtest.BacktestMetrics{
				SharpeDaily: 1.5,
				WinRate:     55,
				TotalTrades: 40,
			},
		},
		{
			Config: backtest.DefaultEngineConfig(),
			Metrics: backtest.BacktestMetrics{
				SharpeDaily: 2.5,
				WinRate:     60,
				TotalTrades: 50,
			},
		},
		{
			Config: backtest.DefaultEngineConfig(),
			Metrics: backtest.BacktestMetrics{
				SharpeDaily: 2.0,
				WinRate:     58,
				TotalTrades: 45,
			},
		},
	}

	best, _ := SelectBest(results)
	if best.Metrics.SharpeDaily != 2.5 {
		t.Errorf("expected Sharpe 2.5, got %f", best.Metrics.SharpeDaily)
	}
}

func TestSelectBestAllFiltered(t *testing.T) {
	// When all are filtered, falls back to highest Sharpe overall.
	results := []GridResult{
		{
			Config: backtest.DefaultEngineConfig(),
			Metrics: backtest.BacktestMetrics{
				SharpeDaily: 5.0,
				WinRate:     90,
				TotalTrades: 5,
			},
		},
		{
			Config: backtest.DefaultEngineConfig(),
			Metrics: backtest.BacktestMetrics{
				SharpeDaily: 6.0,
				WinRate:     95,
				TotalTrades: 3,
			},
		},
	}

	best, _ := SelectBest(results)
	if best.Metrics.SharpeDaily != 6.0 {
		t.Errorf("fallback: expected Sharpe 6.0, got %f", best.Metrics.SharpeDaily)
	}
}

func TestSelectBestEmpty(t *testing.T) {
	best, idx := SelectBest(nil)
	if idx != -1 {
		t.Errorf("expected index -1 for empty, got %d", idx)
	}
	if best.Metrics.SharpeDaily != 0 {
		t.Errorf("expected zero Sharpe for empty, got %f", best.Metrics.SharpeDaily)
	}
}

func TestEvaluateFoldProducesValidResult(t *testing.T) {
	// Create a tiny grid.
	grid := ParamGrid{
		ConfluenceThreshold: ParamRange{"c", 0.50, 0.50, 0.05}, // 1 value
		MinOFIPersistence:   ParamRange{"o", 2, 2, 2},           // 1 value
		StopATRMult:         ParamRange{"s", 1.0, 1.0, 0.5},     // 1 value
		TargetATRMult:       ParamRange{"t", 2.0, 2.0, 0.5},     // 1 value
		MaxHoldingHours:     ParamRange{"h", 1, 1, 1},            // 1 value
	}

	// Create minimal fold data.
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	trainRecords := make([]replay.Record, 50)
	for i := range trainRecords {
		trainRecords[i] = replay.Record{
			LocalTS: start.Add(time.Duration(i) * time.Minute).UnixMilli(),
		}
	}
	valStart := start.Add(50 * time.Minute)
	valRecords := make([]replay.Record, 20)
	for i := range valRecords {
		valRecords[i] = replay.Record{
			LocalTS: valStart.Add(time.Duration(i) * time.Minute).UnixMilli(),
		}
	}
	testStart := valStart.Add(20 * time.Minute)
	testRecords := make([]replay.Record, 20)
	for i := range testRecords {
		testRecords[i] = replay.Record{
			LocalTS: testStart.Add(time.Duration(i) * time.Minute).UnixMilli(),
		}
	}

	fold := Fold{
		Index:      0,
		Train:      trainRecords,
		Val:        valRecords,
		Test:       testRecords,
		TrainStart: start,
		TrainEnd:   valStart,
		ValStart:   valStart,
		ValEnd:     testStart,
		TestStart:  testStart,
		TestEnd:    testStart.Add(20 * time.Minute),
	}

	ctx := context.Background()
	result, err := EvaluateFold(ctx, fold, grid, 1)
	if err != nil {
		t.Fatalf("EvaluateFold error: %v", err)
	}

	if result.FoldIndex != 0 {
		t.Errorf("FoldIndex: got %d, want 0", result.FoldIndex)
	}
	if result.ConfigsEvaluated != 1 {
		t.Errorf("ConfigsEvaluated: got %d, want 1", result.ConfigsEvaluated)
	}
	if result.TrainPeriod == "" {
		t.Error("TrainPeriod is empty")
	}
	if result.ValPeriod == "" {
		t.Error("ValPeriod is empty")
	}
	if result.TestPeriod == "" {
		t.Error("TestPeriod is empty")
	}

	t.Logf("FoldResult: train=%s val=%s test=%s configs=%d",
		result.TrainPeriod, result.ValPeriod, result.TestPeriod, result.ConfigsEvaluated)
}
