package nautilus

import (
	"context"

	pb "github.com/ashark-ai-05/tradefox/internal/nautilus/proto"
)

// Strategy represents a strategy summary from the bridge.
type Strategy struct {
	ID        string
	ClassName string
	State     string
	Params    map[string]string
}

// ListStrategies returns all deployed strategies.
func (b *NautilusBridge) ListStrategies(ctx context.Context) ([]Strategy, error) {
	resp, err := b.strategies.ListStrategies(ctx, &pb.Empty{})
	if err != nil {
		return nil, err
	}

	strategies := make([]Strategy, 0, len(resp.Strategies))
	for _, s := range resp.Strategies {
		strategies = append(strategies, Strategy{
			ID:        s.Id,
			ClassName: s.ClassName,
			State:     s.State,
			Params:    s.Params,
		})
	}
	return strategies, nil
}

// DeployStrategy deploys a new strategy instance.
func (b *NautilusBridge) DeployStrategy(ctx context.Context, class string, params map[string]string, symbols []string, venue string) error {
	resp, err := b.strategies.DeployStrategy(ctx, &pb.DeployRequest{
		StrategyClass: class,
		Params:        params,
		Symbols:       symbols,
		Venue:         venue,
	})
	if err != nil {
		return err
	}
	if !resp.Ok {
		return &BridgeError{Message: resp.Message}
	}
	return nil
}

// StopStrategy stops a running strategy.
func (b *NautilusBridge) StopStrategy(ctx context.Context, id string) error {
	resp, err := b.strategies.StopStrategy(ctx, &pb.StrategyId{Id: id})
	if err != nil {
		return err
	}
	if !resp.Ok {
		return &BridgeError{Message: resp.Message}
	}
	return nil
}

// BridgeError represents an error from the Nautilus bridge.
type BridgeError struct {
	Message string
}

func (e *BridgeError) Error() string {
	return e.Message
}
