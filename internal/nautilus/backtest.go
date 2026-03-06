package nautilus

import (
	"context"
	"io"

	pb "github.com/ashark-ai-05/tradefox/internal/nautilus/proto"
)

// BacktestProgress represents progress of a running backtest.
type BacktestProgress struct {
	BacktestID  string
	PctComplete float64
	Status      string
	Message     string
	TimestampNs int64
}

// BacktestResult represents the outcome of a completed backtest.
type BacktestResult struct {
	ID           string
	TotalReturn  float64
	SharpeRatio  float64
	MaxDrawdown  float64
	WinRate      float64
	TotalTrades  int32
	ProfitFactor float64
}

// BacktestSummary represents a brief summary of a backtest.
type BacktestSummary struct {
	ID            string
	StrategyClass string
	Status        string
	TotalReturn   float64
	SharpeRatio   float64
}

// RunBacktest starts a backtest and streams progress updates.
func (b *NautilusBridge) RunBacktest(ctx context.Context, config *pb.BacktestConfig) (<-chan BacktestProgress, error) {
	stream, err := b.backtest.RunBacktest(ctx, config)
	if err != nil {
		return nil, err
	}

	ch := make(chan BacktestProgress, 64)
	go func() {
		defer close(ch)
		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				b.logger.Warn("backtest stream error", "error", err)
				return
			}
			select {
			case ch <- BacktestProgress{
				BacktestID:  msg.BacktestId,
				PctComplete: msg.PctComplete,
				Status:      msg.Status,
				Message:     msg.Message,
				TimestampNs: msg.TimestampNs,
			}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

// GetResult fetches the result of a completed backtest.
func (b *NautilusBridge) GetResult(ctx context.Context, id string) (*BacktestResult, error) {
	resp, err := b.backtest.GetBacktestResult(ctx, &pb.BacktestId{Id: id})
	if err != nil {
		return nil, err
	}
	return &BacktestResult{
		ID:           resp.Id,
		TotalReturn:  resp.TotalReturn,
		SharpeRatio:  resp.SharpeRatio,
		MaxDrawdown:  resp.MaxDrawdown,
		WinRate:      resp.WinRate,
		TotalTrades:  resp.TotalTrades,
		ProfitFactor: resp.ProfitFactor,
	}, nil
}

// ListBacktests returns summaries of all backtests.
func (b *NautilusBridge) ListBacktests(ctx context.Context) ([]BacktestSummary, error) {
	resp, err := b.backtest.ListBacktests(ctx, &pb.Empty{})
	if err != nil {
		return nil, err
	}

	summaries := make([]BacktestSummary, 0, len(resp.Backtests))
	for _, bt := range resp.Backtests {
		summaries = append(summaries, BacktestSummary{
			ID:            bt.Id,
			StrategyClass: bt.StrategyClass,
			Status:        bt.Status,
			TotalReturn:   bt.TotalReturn,
			SharpeRatio:   bt.SharpeRatio,
		})
	}
	return summaries, nil
}
