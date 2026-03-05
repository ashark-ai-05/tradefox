package models

import (
	"time"

	"github.com/shopspring/decimal"
)

// Trade represents a single market trade.
type Trade struct {
	ProviderID     int             `json:"providerId"`
	ProviderName   string          `json:"providerName"`
	Symbol         string          `json:"symbol"`
	Price          decimal.Decimal `json:"price"`
	Size           decimal.Decimal `json:"size"`
	Timestamp      time.Time       `json:"timestamp"`
	IsBuy          *bool           `json:"isBuy"`
	Flags          string          `json:"flags,omitempty"`
	MarketMidPrice float64         `json:"marketMidPrice"`
}

// CopyTo copies all fields from t into target.
func (t *Trade) CopyTo(target *Trade) {
	target.ProviderID = t.ProviderID
	target.ProviderName = t.ProviderName
	target.Symbol = t.Symbol
	target.Price = t.Price
	target.Size = t.Size
	target.Timestamp = t.Timestamp
	target.IsBuy = copyBoolPtr(t.IsBuy)
	target.Flags = t.Flags
	target.MarketMidPrice = t.MarketMidPrice
}

// copyBoolPtr returns a deep copy of a *bool (nil-safe).
func copyBoolPtr(p *bool) *bool {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
