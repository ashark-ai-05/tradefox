package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strings"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/backtest"
	"github.com/ashark-ai-05/tradefox/internal/replay"
)

func main() {
	var (
		dataDirFlag    = flag.String("data-dir", "data/recorded", "recorded data directory")
		symbolsFlag    = flag.String("symbols", "", "comma-separated symbols (default: all)")
		equityFlag     = flag.Float64("equity", 10000, "initial equity in USDT")
		confluenceFlag = flag.Float64("confluence", 0.60, "confluence threshold")
		outputFlag     = flag.String("output", "text", "output format: text, json")
		outputFile     = flag.String("output-file", "", "output file path (default: stdout)")
		tradesFile     = flag.String("trades-file", "", "export trades to JSON (optional)")
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

	if *symbolsFlag != "" {
		syms := strings.Split(*symbolsFlag, ",")
		set := map[string]bool{}
		for _, s := range syms {
			set[strings.TrimSpace(s)] = true
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
			if set[sym] {
				filtered = append(filtered, r)
			}
		}
		records = filtered
		logger.Info("filtered", slog.Int("count", len(records)))
	}

	cfg := backtest.DefaultEngineConfig()
	cfg.InitialEquity = *equityFlag
	cfg.Strategy.ConfluenceThreshold = *confluenceFlag

	logger.Info("running backtest")
	engine := backtest.NewEngine(cfg, logger)
	result, err := engine.Run(records)
	if err != nil {
		fmt.Fprintf(os.Stderr, "backtest failed: %v\n", err)
		os.Exit(1)
	}

	overfit := backtest.CheckOverfitting(result)

	switch *outputFlag {
	case "json":
		out := struct {
			*backtest.BacktestResult
			Overfit *backtest.OverfitCheck `json:"overfit"`
		}{result, overfit}
		data, _ := json.MarshalIndent(out, "", "  ")
		if *outputFile != "" {
			os.WriteFile(*outputFile, data, 0o644)
		} else {
			fmt.Println(string(data))
		}
	default:
		printReport(result, overfit)
	}

	if *tradesFile != "" {
		data, _ := json.MarshalIndent(result.Trades, "", "  ")
		os.WriteFile(*tradesFile, data, 0o644)
		logger.Info("trades exported", slog.String("file", *tradesFile))
	}

	logger.Info("backtest complete")
}

func printReport(result *backtest.BacktestResult, overfit *backtest.OverfitCheck) {
	m := result.Metrics
	s := result.DataStats
	fmt.Println("=====================================================")
	fmt.Println(" BACKTEST RESULTS")
	fmt.Println("=====================================================")
	if s.StartTime > 0 {
		fmt.Printf(" Period:          %s -> %s\n",
			time.UnixMilli(s.StartTime).UTC().Format("2006-01-02"),
			time.UnixMilli(s.EndTime).UTC().Format("2006-01-02"))
	}
	fmt.Printf(" Symbols:         %s\n", strings.Join(s.Symbols, ", "))
	fmt.Printf(" Initial Equity:  $%.2f\n", result.Config.InitialEquity)
	fmt.Printf(" Data:            %d OB, %d trades, %d kiy\n", s.OBRecords, s.TradeRecords, s.KiyRecords)
	fmt.Println("-----------------------------------------------------")
	fmt.Printf(" Total Trades:    %d\n", m.TotalTrades)
	fmt.Printf(" Win Rate:        %.1f%%\n", m.WinRate)
	fmt.Printf(" Avg Win:         %.1f bps\n", m.AvgWinPct*100)
	fmt.Printf(" Avg Loss:        %.1f bps\n", math.Abs(m.AvgLossPct)*100)
	fmt.Printf(" Profit Factor:   %.2f\n", m.ProfitFactor)
	fmt.Printf(" Sharpe (daily):  %.1f\n", m.SharpeDaily)
	fmt.Printf(" Max Drawdown:    %.1f%%\n", m.MaxDrawdownPct)
	fmt.Printf(" Total Return:    %.1f%%\n", m.TotalReturnPct)
	fmt.Printf(" Trades/Day:      %.1f\n", m.TradesPerDay)
	if m.AvgHoldingMs > 0 {
		fmt.Printf(" Avg Holding:     %s\n", time.Duration(m.AvgHoldingMs)*time.Millisecond)
	}
	if len(m.BySymbol) > 1 {
		fmt.Println("-----------------------------------------------------")
		for sym, sm := range m.BySymbol {
			fmt.Printf("   %-12s  %d trades  %.1f%% win  $%.2f\n", sym, sm.Trades, sm.WinRate, sm.TotalPnL)
		}
	}
	fmt.Println("-----------------------------------------------------")
	fmt.Printf(" OVERFIT CHECK: [%s]\n", strings.ToUpper(overfit.Verdict))
	for _, f := range overfit.Flags {
		fmt.Printf("   WARNING: %s\n", f)
	}
	for _, h := range overfit.Healthy {
		fmt.Printf("   OK: %s\n", h)
	}
	fmt.Println("=====================================================")
}
