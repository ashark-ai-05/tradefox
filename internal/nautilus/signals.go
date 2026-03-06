package nautilus

import (
	"context"
	"io"

	pb "github.com/ashark-ai-05/tradefox/internal/nautilus/proto"
)

// Signal represents a strategy signal from NautilusTrader.
type Signal struct {
	StrategyID  string
	SignalType  string
	Symbol      string
	Side        string
	Price       float64
	Size        float64
	Confidence  float64
	TimestampNs int64
	Metadata    map[string]string
}

// StreamSignals opens a server-streaming RPC for strategy signals.
// Returns a channel that receives signals until the stream ends or context is canceled.
func (b *NautilusBridge) StreamSignals(ctx context.Context, strategyID string) (<-chan Signal, error) {
	stream, err := b.strategies.StreamSignals(ctx, &pb.StrategyId{Id: strategyID})
	if err != nil {
		return nil, err
	}

	ch := make(chan Signal, 64)
	go func() {
		defer close(ch)
		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				b.logger.Warn("signal stream error", "error", err)
				return
			}
			sig := Signal{
				StrategyID:  msg.StrategyId,
				SignalType:   msg.SignalType,
				Symbol:      msg.Symbol,
				Side:        msg.Side,
				Price:       msg.Price,
				Size:        msg.Size,
				Confidence:  msg.Confidence,
				TimestampNs: msg.TimestampNs,
				Metadata:    msg.Metadata,
			}
			select {
			case ch <- sig:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}
