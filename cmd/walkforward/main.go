package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/ashark-ai-05/tradefox/internal/replay"
	"github.com/ashark-ai-05/tradefox/internal/walkforward"
)

func main() {
	var (
		dataDirFlag = flag.String("data-dir", "data/recorded", "recorded data directory")
		symbolsFlag = flag.String("symbols", "", "comma-separated symbols (default: all)")
		workersFlag = flag.Int("workers", 4, "parallel backtest workers")
		outputFlag  = flag.String("output", "text", "output format: text, json")
		outputFile  = flag.String("output-file", "", "output file path (default: stdout)")
		equityFlag  = flag.Float64("equity", 10000, "initial equity in USDT")
	)
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// Set up context with signal handling.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 1. Load recorded data.
	logger.Info("loading recorded data", "dir", *dataDirFlag)
	records, err := replay.ReadAllData(*dataDirFlag)
	if err != nil {
		logger.Error("failed to load data", "error", err)
		os.Exit(1)
	}
	if len(records) == 0 {
		logger.Error("no records found", "dir", *dataDirFlag)
		os.Exit(1)
	}
	logger.Info("loaded records", "count", len(records))

	// 2. Filter by symbols if specified.
	if *symbolsFlag != "" {
		symbols := strings.Split(*symbolsFlag, ",")
		symbolSet := make(map[string]bool, len(symbols))
		for _, s := range symbols {
			symbolSet[strings.TrimSpace(s)] = true
		}
		records = filterBySymbols(records, symbolSet)
		logger.Info("filtered by symbols", "symbols", *symbolsFlag, "remaining", len(records))
	}

	// 3. Create WalkForwardConfig with defaults, override equity/workers.
	cfg := walkforward.DefaultWalkForwardConfig()
	cfg.Workers = *workersFlag
	cfg.InitialEquity = *equityFlag

	// 4. Run walk-forward optimization.
	logger.Info("starting walk-forward optimization",
		"gridSize", cfg.Grid.Count(),
		"workers", cfg.Workers,
		"equity", cfg.InitialEquity,
	)

	result, err := walkforward.RunWalkForward(ctx, records, cfg)
	if err != nil {
		logger.Error("walk-forward failed", "error", err)
		os.Exit(1)
	}

	// 5. Output results.
	if err := writeOutput(result, *outputFlag, *outputFile); err != nil {
		logger.Error("output failed", "error", err)
		os.Exit(1)
	}
}

func filterBySymbols(records []replay.Record, symbols map[string]bool) []replay.Record {
	var filtered []replay.Record
	for _, r := range records {
		var sym string
		switch {
		case r.OB != nil:
			sym = r.OB.Symbol
		case r.Trade != nil:
			sym = r.Trade.Symbol
		case r.Kiy != nil:
			sym = r.Kiy.Symbol
		default:
			continue
		}
		if symbols[sym] {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

func writeOutput(result *walkforward.WalkForwardResult, format, filePath string) error {
	switch format {
	case "json":
		if filePath != "" {
			return result.WriteJSON(filePath)
		}
		// Write JSON to stdout.
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)

	case "text":
		if filePath != "" {
			f, err := os.Create(filePath)
			if err != nil {
				return fmt.Errorf("create %s: %w", filePath, err)
			}
			defer f.Close()
			result.WriteSummary(f)
			return nil
		}
		result.WriteSummary(os.Stdout)
		return nil

	default:
		return fmt.Errorf("unknown output format: %s (use 'text' or 'json')", format)
	}
}
