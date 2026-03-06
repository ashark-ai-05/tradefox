package nautilus

import (
	"context"
	"io"

	pb "github.com/ashark-ai-05/tradefox/internal/nautilus/proto"
)

// OrderRequest represents a new order to submit.
type OrderRequest struct {
	Symbol        string
	Side          string
	OrderType     string
	Quantity      float64
	Price         float64
	TimeInForce   string
	StopLoss      float64
	TakeProfit    float64
	StrategyID    string
	ClientOrderID string
}

// OrderResponse represents the result of an order submission.
type OrderResponse struct {
	OK            bool
	OrderID       string
	ClientOrderID string
	Status        string
	Message       string
}

// OrderUpdate represents a real-time order status change.
type OrderUpdate struct {
	OrderID       string
	ClientOrderID string
	Symbol        string
	Status        string
	Side          string
	Price         float64
	Quantity      float64
	FilledQty     float64
	AvgPrice      float64
	TimestampNs   int64
}

// PositionUpdate represents a real-time position change.
type PositionUpdate struct {
	Symbol        string
	Side          string
	Quantity      float64
	AvgEntry      float64
	UnrealizedPnL float64
	RealizedPnL   float64
	TimestampNs   int64
}

// SubmitOrder submits a new order via gRPC.
func (b *NautilusBridge) SubmitOrder(ctx context.Context, req OrderRequest) (*OrderResponse, error) {
	resp, err := b.execution.SubmitOrder(ctx, &pb.OrderRequest{
		Symbol:        req.Symbol,
		Side:          req.Side,
		OrderType:     req.OrderType,
		Quantity:      req.Quantity,
		Price:         req.Price,
		TimeInForce:   req.TimeInForce,
		StopLoss:      req.StopLoss,
		TakeProfit:    req.TakeProfit,
		StrategyId:    req.StrategyID,
		ClientOrderId: req.ClientOrderID,
	})
	if err != nil {
		return nil, err
	}
	return &OrderResponse{
		OK:            resp.Ok,
		OrderID:       resp.OrderId,
		ClientOrderID: resp.ClientOrderId,
		Status:        resp.Status,
		Message:       resp.Message,
	}, nil
}

// CancelOrder cancels an existing order.
func (b *NautilusBridge) CancelOrder(ctx context.Context, orderID, symbol string) error {
	resp, err := b.execution.CancelOrder(ctx, &pb.CancelRequest{
		OrderId: orderID,
		Symbol:  symbol,
	})
	if err != nil {
		return err
	}
	if !resp.Ok {
		return &BridgeError{Message: resp.Message}
	}
	return nil
}

// ModifyOrder modifies an existing order.
func (b *NautilusBridge) ModifyOrder(ctx context.Context, orderID, symbol string, newPrice, newQty, newSL, newTP float64) (*OrderResponse, error) {
	resp, err := b.execution.ModifyOrder(ctx, &pb.ModifyRequest{
		OrderId:       orderID,
		Symbol:        symbol,
		NewPrice:      newPrice,
		NewQuantity:   newQty,
		NewStopLoss:   newSL,
		NewTakeProfit: newTP,
	})
	if err != nil {
		return nil, err
	}
	return &OrderResponse{
		OK:            resp.Ok,
		OrderID:       resp.OrderId,
		ClientOrderID: resp.ClientOrderId,
		Status:        resp.Status,
		Message:       resp.Message,
	}, nil
}

// StreamOrderUpdates opens a stream for real-time order updates.
func (b *NautilusBridge) StreamOrderUpdates(ctx context.Context) (<-chan OrderUpdate, error) {
	stream, err := b.execution.StreamOrderUpdates(ctx, &pb.Empty{})
	if err != nil {
		return nil, err
	}

	ch := make(chan OrderUpdate, 64)
	go func() {
		defer close(ch)
		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				b.logger.Warn("order update stream error", "error", err)
				return
			}
			select {
			case ch <- OrderUpdate{
				OrderID:       msg.OrderId,
				ClientOrderID: msg.ClientOrderId,
				Symbol:        msg.Symbol,
				Status:        msg.Status,
				Side:          msg.Side,
				Price:         msg.Price,
				Quantity:      msg.Quantity,
				FilledQty:     msg.FilledQty,
				AvgPrice:      msg.AvgPrice,
				TimestampNs:   msg.TimestampNs,
			}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

// StreamPositionUpdates opens a stream for real-time position updates.
func (b *NautilusBridge) StreamPositionUpdates(ctx context.Context) (<-chan PositionUpdate, error) {
	stream, err := b.execution.StreamPositionUpdates(ctx, &pb.Empty{})
	if err != nil {
		return nil, err
	}

	ch := make(chan PositionUpdate, 64)
	go func() {
		defer close(ch)
		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				b.logger.Warn("position update stream error", "error", err)
				return
			}
			select {
			case ch <- PositionUpdate{
				Symbol:        msg.Symbol,
				Side:          msg.Side,
				Quantity:      msg.Quantity,
				AvgEntry:      msg.AvgEntry,
				UnrealizedPnL: msg.UnrealizedPnl,
				RealizedPnL:   msg.RealizedPnl,
				TimestampNs:   msg.TimestampNs,
			}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}
