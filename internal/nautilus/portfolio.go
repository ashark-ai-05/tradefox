package nautilus

import (
	"context"
	"io"

	pb "github.com/ashark-ai-05/tradefox/internal/nautilus/proto"
)

// Portfolio represents portfolio state.
type Portfolio struct {
	TotalEquity      float64
	AvailableBalance float64
	MarginUsed       float64
	Currency         string
	Positions        []Position
}

// Position represents a single position.
type Position struct {
	Symbol        string
	Side          string
	Quantity      float64
	AvgEntry      float64
	MarkPrice     float64
	UnrealizedPnL float64
	Margin        float64
}

// RiskMetrics represents current risk state.
type RiskMetrics struct {
	TotalEquity      float64
	UnrealizedPnL    float64
	RealizedPnL      float64
	MaxDrawdown      float64
	DailyPnL         float64
	DailyLossLimit   float64
	DailyLossUsed    float64
	KillSwitchActive bool
}

// PnLUpdate represents a real-time P&L change.
type PnLUpdate struct {
	TotalEquity   float64
	UnrealizedPnL float64
	RealizedPnL   float64
	DailyPnL      float64
	TimestampNs   int64
}

// GetPortfolio returns the current portfolio state.
func (b *NautilusBridge) GetPortfolio(ctx context.Context) (*Portfolio, error) {
	resp, err := b.portfolio.GetPortfolio(ctx, &pb.Empty{})
	if err != nil {
		return nil, err
	}

	positions := make([]Position, 0, len(resp.Positions))
	for _, p := range resp.Positions {
		positions = append(positions, Position{
			Symbol:        p.Symbol,
			Side:          p.Side,
			Quantity:      p.Quantity,
			AvgEntry:      p.AvgEntry,
			MarkPrice:     p.MarkPrice,
			UnrealizedPnL: p.UnrealizedPnl,
			Margin:        p.Margin,
		})
	}

	return &Portfolio{
		TotalEquity:      resp.TotalEquity,
		AvailableBalance: resp.AvailableBalance,
		MarginUsed:       resp.MarginUsed,
		Currency:         resp.Currency,
		Positions:        positions,
	}, nil
}

// GetPositions returns filtered positions.
func (b *NautilusBridge) GetPositions(ctx context.Context) ([]Position, error) {
	resp, err := b.portfolio.GetPositions(ctx, &pb.PositionFilter{OpenOnly: true})
	if err != nil {
		return nil, err
	}

	positions := make([]Position, 0, len(resp.Positions))
	for _, p := range resp.Positions {
		positions = append(positions, Position{
			Symbol:        p.Symbol,
			Side:          p.Side,
			Quantity:      p.Quantity,
			AvgEntry:      p.AvgEntry,
			MarkPrice:     p.MarkPrice,
			UnrealizedPnL: p.UnrealizedPnl,
			Margin:        p.Margin,
		})
	}
	return positions, nil
}

// GetRisk returns current risk metrics.
func (b *NautilusBridge) GetRisk(ctx context.Context) (*RiskMetrics, error) {
	resp, err := b.portfolio.GetRiskMetrics(ctx, &pb.Empty{})
	if err != nil {
		return nil, err
	}
	return &RiskMetrics{
		TotalEquity:      resp.TotalEquity,
		UnrealizedPnL:    resp.UnrealizedPnl,
		RealizedPnL:      resp.RealizedPnl,
		MaxDrawdown:      resp.MaxDrawdown,
		DailyPnL:         resp.DailyPnl,
		DailyLossLimit:   resp.DailyLossLimit,
		DailyLossUsed:    resp.DailyLossUsed,
		KillSwitchActive: resp.KillSwitchActive,
	}, nil
}

// StreamPnL opens a stream for real-time P&L updates.
func (b *NautilusBridge) StreamPnL(ctx context.Context) (<-chan PnLUpdate, error) {
	stream, err := b.portfolio.StreamPnL(ctx, &pb.Empty{})
	if err != nil {
		return nil, err
	}

	ch := make(chan PnLUpdate, 64)
	go func() {
		defer close(ch)
		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				b.logger.Warn("pnl stream error", "error", err)
				return
			}
			select {
			case ch <- PnLUpdate{
				TotalEquity:   msg.TotalEquity,
				UnrealizedPnL: msg.UnrealizedPnl,
				RealizedPnL:   msg.RealizedPnl,
				DailyPnL:      msg.DailyPnl,
				TimestampNs:   msg.TimestampNs,
			}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}
