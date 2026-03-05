package models

import (
	"time"

	"github.com/ashark-ai-05/tradefox/internal/core/enums"
)

// Execution represents a trade execution report.
type Execution struct {
	OrderID         int64             `json:"orderId"`
	ExecutionID     string            `json:"executionId"`
	Price           float64           `json:"price"`
	QtyFilled       float64           `json:"qtyFilled"`
	Timestamp       time.Time         `json:"timestamp"`
	Side            enums.OrderSide   `json:"side"`
	Status          enums.OrderStatus `json:"status"`
	ServerTimestamp time.Time         `json:"serverTimestamp"`
	LocalTimestamp  time.Time         `json:"localTimestamp"`
}

// Latency returns the duration between LocalTimestamp and ServerTimestamp
// (i.e., the observed network + processing latency).
func (e *Execution) Latency() time.Duration {
	return e.LocalTimestamp.Sub(e.ServerTimestamp)
}
