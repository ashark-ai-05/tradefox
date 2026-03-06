package walkforward

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/ashark-ai-05/tradefox/internal/backtest"
)

// WriteJSON writes the WalkForwardResult as JSON to the specified path.
func (r *WalkForwardResult) WriteJSON(path string) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// WriteSummary writes a human-readable summary to the provided writer.
func (r *WalkForwardResult) WriteSummary(w io.Writer) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  WALK-FORWARD OPTIMIZATION RESULTS")
	fmt.Fprintln(w, "")

	fmt.Fprintf(w, "  Folds: %d  |  Total Test Trades: %d\n",
		r.Summary.NumFolds, r.Summary.TotalTestTrades)
	fmt.Fprintln(w, "")

	for i, f := range r.Folds {
		fmt.Fprintf(w, "  --- Fold %d ---\n", i+1)
		fmt.Fprintf(w, "  Train: %s\n", f.TrainPeriod)
		fmt.Fprintf(w, "  Val:   %s\n", f.ValPeriod)
		fmt.Fprintf(w, "  Test:  %s\n", f.TestPeriod)
		fmt.Fprintf(w, "  Best Config: Confluence=%.2f OFIPersist=%d Stop=%.1fATR Target=%.1fATR MaxHold=%dh\n",
			f.BestConfig.Strategy.ConfluenceThreshold,
			f.BestConfig.Strategy.MinOFIPersistence,
			f.BestConfig.Position.StopATRMult,
			f.BestConfig.Position.TargetATRMult,
			f.BestConfig.Position.MaxHoldingMs/(60*60*1000),
		)
		fmt.Fprintf(w, "  Train -> Sharpe: %.1f  WinRate: %.1f%%  Trades: %d\n",
			f.TrainMetrics.SharpeDaily, f.TrainMetrics.WinRate, f.TrainMetrics.TotalTrades)
		fmt.Fprintf(w, "  Val   -> Sharpe: %.1f  WinRate: %.1f%%  Trades: %d\n",
			f.ValMetrics.SharpeDaily, f.ValMetrics.WinRate, f.ValMetrics.TotalTrades)
		fmt.Fprintf(w, "  Test  -> Sharpe: %.1f  WinRate: %.1f%%  Trades: %d\n",
			f.TestMetrics.SharpeDaily, f.TestMetrics.WinRate, f.TestMetrics.TotalTrades)
		fmt.Fprintln(w, "")
	}

	fmt.Fprintln(w, "  --- Summary ---")
	fmt.Fprintf(w, "  Avg Test Sharpe:    %.1f\n", r.Summary.AvgTestSharpe)
	fmt.Fprintf(w, "  Avg Test Win Rate:  %.1f%%\n", r.Summary.AvgTestWinRate)
	fmt.Fprintf(w, "  Train/Test Ratio:   %.1fx\n", r.Summary.TrainTestRatio)
	fmt.Fprintf(w, "  Total Test Trades:  %d\n", r.Summary.TotalTestTrades)
	fmt.Fprintln(w, "")

	fmt.Fprintln(w, "  --- Stability ---")
	for _, ps := range r.Stability.ParamStats {
		fmt.Fprintf(w, "  %-22s %.2f +/- %.2f (CV: %.0f%%)\n",
			ps.Name+":", ps.Mean, ps.StdDev, ps.CV*100)
	}
	if len(r.Stability.Flags) > 0 {
		fmt.Fprintln(w, "")
		for _, flag := range r.Stability.Flags {
			fmt.Fprintf(w, "  [!] %s\n", flag)
		}
	}
	fmt.Fprintf(w, "  Verdict: %s\n", r.Stability.Verdict)
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "")
}

// RecommendedConfig extracts the consensus best config across folds.
// Uses the median of each parameter's best values across folds.
func (r *WalkForwardResult) RecommendedConfig() backtest.EngineConfig {
	cfg := backtest.DefaultEngineConfig()

	if len(r.Folds) == 0 {
		return cfg
	}

	// Collect per-fold parameter values.
	n := len(r.Folds)
	confluenceVals := make([]float64, n)
	ofiVals := make([]float64, n)
	stopVals := make([]float64, n)
	targetVals := make([]float64, n)
	holdVals := make([]float64, n)

	for i, f := range r.Folds {
		confluenceVals[i] = f.BestConfig.Strategy.ConfluenceThreshold
		ofiVals[i] = float64(f.BestConfig.Strategy.MinOFIPersistence)
		stopVals[i] = f.BestConfig.Position.StopATRMult
		targetVals[i] = f.BestConfig.Position.TargetATRMult
		holdVals[i] = float64(f.BestConfig.Position.MaxHoldingMs)
	}

	cfg.Strategy.ConfluenceThreshold = median(confluenceVals)
	cfg.Strategy.MinOFIPersistence = int(median(ofiVals))
	cfg.Position.StopATRMult = median(stopVals)
	cfg.Position.TargetATRMult = median(targetVals)
	cfg.Position.MaxHoldingMs = int64(median(holdVals))

	return cfg
}

// WriteRecommended writes the recommended config as a JSON file.
func (r *WalkForwardResult) WriteRecommended(path string) error {
	cfg := r.RecommendedConfig()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal recommended config: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
