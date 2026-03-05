package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/ashark-ai-05/tradefox/internal/replay"
	"github.com/ashark-ai-05/tradefox/internal/validate"
)

func main() {
	var (
		dataDirFlag = flag.String("data-dir", "data/recorded", "recorded data directory")
		symbolsFlag = flag.String("symbols", "", "comma-separated symbols to validate (default: all)")
		outputFlag  = flag.String("output", "text", "output format: text, json")
		outputFile  = flag.String("output-file", "", "output file path (default: stdout)")
	)
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	logger.Info("loading recorded data", slog.String("dir", *dataDirFlag))
	records, err := replay.ReadAllData(*dataDirFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read data: %v\n", err)
		os.Exit(1)
	}
	logger.Info("loaded records", slog.Int("count", len(records)))

	// Filter by symbols if specified
	if *symbolsFlag != "" {
		symbols := strings.Split(*symbolsFlag, ",")
		symbolSet := map[string]bool{}
		for _, s := range symbols {
			symbolSet[strings.TrimSpace(s)] = true
		}
		var filtered []replay.Record
		for _, r := range records {
			sym := ""
			if r.OB != nil {
				sym = r.OB.Symbol
			}
			if r.Trade != nil {
				sym = r.Trade.Symbol
			}
			if r.Kiy != nil {
				sym = r.Kiy.Symbol
			}
			if symbolSet[sym] {
				filtered = append(filtered, r)
			}
		}
		records = filtered
		logger.Info("filtered to symbols", slog.String("symbols", *symbolsFlag), slog.Int("count", len(records)))
	}

	// Replay through signals
	logger.Info("replaying through signal engine")
	replayer := validate.NewReplayer()
	var snapshots []validate.SignalSnapshot
	for _, rec := range records {
		if snap := replayer.Process(rec); snap != nil {
			snapshots = append(snapshots, *snap)
		}
	}
	logger.Info("signal snapshots generated", slog.Int("count", len(snapshots)))

	// Compute forward returns
	logger.Info("computing forward returns")
	rows := validate.ComputeForwardReturns(snapshots, validate.DefaultHorizons)

	// Generate report
	logger.Info("generating validation report")
	report := validate.GenerateReport(rows, records)

	// Output
	switch *outputFlag {
	case "json":
		if *outputFile != "" {
			if err := report.WriteJSON(*outputFile); err != nil {
				fmt.Fprintf(os.Stderr, "failed to write JSON: %v\n", err)
				os.Exit(1)
			}
		} else {
			data, _ := json.MarshalIndent(report, "", "  ")
			fmt.Fprintln(os.Stdout, string(data))
		}
	default:
		if *outputFile != "" {
			f, err := os.Create(*outputFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to create output file: %v\n", err)
				os.Exit(1)
			}
			defer f.Close()
			report.WriteSummary(f)
		} else {
			report.WriteSummary(os.Stdout)
		}
	}

	logger.Info("validation complete")
}
