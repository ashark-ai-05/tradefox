package models

import (
	"time"

	"github.com/ashark-ai-05/tradefox/internal/core/enums"
)

// Order represents a trading order tracking the full lifecycle from placement
// through fills, cancellation, or rejection. It mirrors the C# Order model
// from VisualHFT.
type Order struct {
	OrderID              int64                  `json:"orderId"`
	Symbol               string                 `json:"symbol"`
	ProviderID           int                    `json:"providerId"`
	ProviderName         string                 `json:"providerName"`
	ClOrdID              string                 `json:"clOrdId,omitempty"`
	Side                 enums.OrderSide        `json:"side"`
	OrderType            enums.OrderType        `json:"orderType"`
	TimeInForce          enums.OrderTimeInForce `json:"timeInForce"`
	Status               enums.OrderStatus      `json:"status"`
	Quantity             float64                `json:"quantity"`
	MinQuantity          float64                `json:"minQuantity,omitempty"`
	FilledQuantity       float64                `json:"filledQuantity"`
	PricePlaced          float64                `json:"pricePlaced"`
	Currency             string                 `json:"currency,omitempty"`
	FutSettDate          string                 `json:"futSettDate,omitempty"`
	IsMM                 bool                   `json:"isMM,omitempty"`
	IsEmpty              bool                   `json:"isEmpty"`
	LayerName            string                 `json:"layerName,omitempty"`
	AttemptsToClose      int                    `json:"attemptsToClose,omitempty"`
	SymbolMultiplier     int                    `json:"symbolMultiplier,omitempty"`
	SymbolDecimals       int                    `json:"symbolDecimals,omitempty"`
	FreeText             string                 `json:"freeText,omitempty"`
	OriginPartyID        string                 `json:"originPartyId,omitempty"`
	Executions           []Execution            `json:"executions,omitempty"`
	QuoteID              int                    `json:"quoteId,omitempty"`
	QuoteServerTimestamp time.Time              `json:"quoteServerTimestamp,omitempty"`
	QuoteLocalTimestamp  time.Time              `json:"quoteLocalTimestamp,omitempty"`
	CreationTimestamp    time.Time              `json:"creationTimestamp"`
	LastUpdated          time.Time              `json:"lastUpdated"`
	ExecutedTimestamp    time.Time              `json:"executedTimestamp,omitempty"`
	FireSignalTimestamp  time.Time              `json:"fireSignalTimestamp,omitempty"`
	StopLoss             float64                `json:"stopLoss,omitempty"`
	TakeProfit           float64                `json:"takeProfit,omitempty"`
	PipsTrail            bool                   `json:"pipsTrail,omitempty"`
	UnrealizedPnL        float64                `json:"unrealizedPnL,omitempty"`
	MaxDrawdown          float64                `json:"maxDrawdown,omitempty"`
	BestBid              float64                `json:"bestBid,omitempty"`
	BestAsk              float64                `json:"bestAsk,omitempty"`
	FilledPrice          float64                `json:"filledPrice,omitempty"`
	FilledPercentage     float64                `json:"filledPercentage,omitempty"`

	// Execution engine fields
	Leverage             float64                `json:"leverage,omitempty"`
	StopPrice            float64                `json:"stopPrice,omitempty"`
	Fees                 float64                `json:"fees,omitempty"`
	Exchange             string                 `json:"exchange,omitempty"`
}

// PendingQuantity returns the unfilled quantity remaining on this order.
// If the order is in a terminal state (Canceled, Rejected, CanceledSent, or
// Filled), it returns 0.
func (o *Order) PendingQuantity() float64 {
	switch o.Status {
	case enums.OrderStatusCanceled,
		enums.OrderStatusRejected,
		enums.OrderStatusCanceledSent,
		enums.OrderStatusFilled:
		return 0
	default:
		return o.Quantity - o.FilledQuantity
	}
}

// Update copies all fields from another order into this one.
func (o *Order) Update(other *Order) {
	o.OrderID = other.OrderID
	o.Symbol = other.Symbol
	o.ProviderID = other.ProviderID
	o.ProviderName = other.ProviderName
	o.ClOrdID = other.ClOrdID
	o.Side = other.Side
	o.OrderType = other.OrderType
	o.TimeInForce = other.TimeInForce
	o.Status = other.Status
	o.Quantity = other.Quantity
	o.MinQuantity = other.MinQuantity
	o.FilledQuantity = other.FilledQuantity
	o.PricePlaced = other.PricePlaced
	o.Currency = other.Currency
	o.FutSettDate = other.FutSettDate
	o.IsMM = other.IsMM
	o.IsEmpty = other.IsEmpty
	o.LayerName = other.LayerName
	o.AttemptsToClose = other.AttemptsToClose
	o.SymbolMultiplier = other.SymbolMultiplier
	o.SymbolDecimals = other.SymbolDecimals
	o.FreeText = other.FreeText
	o.OriginPartyID = other.OriginPartyID
	o.QuoteID = other.QuoteID
	o.QuoteServerTimestamp = other.QuoteServerTimestamp
	o.QuoteLocalTimestamp = other.QuoteLocalTimestamp
	o.CreationTimestamp = other.CreationTimestamp
	o.LastUpdated = other.LastUpdated
	o.ExecutedTimestamp = other.ExecutedTimestamp
	o.FireSignalTimestamp = other.FireSignalTimestamp
	o.StopLoss = other.StopLoss
	o.TakeProfit = other.TakeProfit
	o.PipsTrail = other.PipsTrail
	o.UnrealizedPnL = other.UnrealizedPnL
	o.MaxDrawdown = other.MaxDrawdown
	o.BestBid = other.BestBid
	o.BestAsk = other.BestAsk
	o.FilledPrice = other.FilledPrice
	o.FilledPercentage = other.FilledPercentage
	o.Leverage = other.Leverage
	o.StopPrice = other.StopPrice
	o.Fees = other.Fees
	o.Exchange = other.Exchange

	// Deep copy the executions slice.
	if other.Executions != nil {
		o.Executions = make([]Execution, len(other.Executions))
		copy(o.Executions, other.Executions)
	} else {
		o.Executions = nil
	}
}

// Reset zeroes all fields to their default values.
func (o *Order) Reset() {
	*o = Order{}
}

// Clone returns a deep copy of this order.
func (o *Order) Clone() *Order {
	clone := *o
	if o.Executions != nil {
		clone.Executions = make([]Execution, len(o.Executions))
		copy(clone.Executions, o.Executions)
	}
	return &clone
}
