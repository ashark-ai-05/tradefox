package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/backfill"
)

func main() {
	var (
		symbolsFlag = flag.String("symbols", "SOLUSDT,ETHUSDT,BTCUSDT", "comma-separated symbols")
		startFlag   = flag.String("start", "", "start date (YYYY-MM-DD), default: 8 weeks ago")
		endFlag     = flag.String("end", "", "end date (YYYY-MM-DD), default: now")
		dataDirFlag = flag.String("data-dir", "data/recorded", "output directory for recorded data")
	)
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	symbols := strings.Split(*symbolsFlag, ",")

	end := time.Now().UTC()
	if *endFlag != "" {
		var err error
		end, err = time.Parse("2006-01-02", *endFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid end date: %v\n", err)
			os.Exit(1)
		}
	}

	start := end.Add(-8 * 7 * 24 * time.Hour) // 8 weeks ago
	if *startFlag != "" {
		var err error
		start, err = time.Parse("2006-01-02", *startFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid start date: %v\n", err)
			os.Exit(1)
		}
	}

	logger.Info("starting backfill",
		slog.String("symbols", strings.Join(symbols, ",")),
		slog.String("start", start.Format("2006-01-02")),
		slog.String("end", end.Format("2006-01-02")),
		slog.String("dataDir", *dataDirFlag),
	)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	orch := backfill.NewOrchestrator(backfill.Config{
		Symbols: symbols,
		DataDir: *dataDirFlag,
		Start:   start,
		End:     end,
	}, logger)

	if err := orch.Run(ctx); err != nil {
		logger.Error("backfill failed", slog.Any("error", err))
		os.Exit(1)
	}

	logger.Info("backfill complete")
}
