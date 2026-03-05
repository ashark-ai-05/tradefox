package models

import (
	"time"

	"github.com/shopspring/decimal"
)

// BaseStudyModel represents a single data point produced by a study/indicator.
type BaseStudyModel struct {
	Value          decimal.Decimal `json:"value"`
	MarketMidPrice float64         `json:"marketMidPrice"`
	Timestamp      time.Time       `json:"timestamp"`
	ValueFormatted string          `json:"valueFormatted,omitempty"`
	ValueColor     string          `json:"valueColor,omitempty"`
	Tooltip        string          `json:"tooltip,omitempty"`

	// IsStale indicates that no data has been received for an extended period.
	IsStale bool `json:"isStale,omitempty"`
	// HasError indicates the study encountered an error condition.
	HasError bool `json:"hasError,omitempty"`

	// AddItemSkippingAggregation bypasses time-bucketed aggregation when true.
	// This field is not serialized; it is used internally by the study pipeline.
	AddItemSkippingAggregation bool `json:"-"`
}
