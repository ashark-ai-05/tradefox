// Package models defines the core domain types used throughout VisualHFT.
package models

import (
	"fmt"
	"math"
	"time"
)

// BookItem represents a single price level entry in a limit order book.
type BookItem struct {
	Symbol             string    `json:"symbol"`
	ProviderID         int       `json:"providerId"`
	EntryID            string    `json:"entryId"`
	LayerName          string    `json:"layerName,omitempty"`
	LocalTimestamp      time.Time `json:"localTimestamp"`
	ServerTimestamp     time.Time `json:"serverTimestamp"`
	Price              *float64  `json:"price"`
	Size               *float64  `json:"size"`
	IsBid              bool      `json:"isBid"`
	CumulativeSize     *float64  `json:"cumulativeSize,omitempty"`
	ActiveSize         *float64  `json:"activeSize,omitempty"`
	PriceDecimalPlaces int       `json:"priceDecimalPlaces"`
	SizeDecimalPlaces  int       `json:"sizeDecimalPlaces"`
}

// CopyFrom copies all fields from other into b.
func (b *BookItem) CopyFrom(other *BookItem) {
	b.Symbol = other.Symbol
	b.ProviderID = other.ProviderID
	b.EntryID = other.EntryID
	b.LayerName = other.LayerName
	b.LocalTimestamp = other.LocalTimestamp
	b.ServerTimestamp = other.ServerTimestamp
	b.Price = copyFloat64Ptr(other.Price)
	b.Size = copyFloat64Ptr(other.Size)
	b.IsBid = other.IsBid
	b.CumulativeSize = copyFloat64Ptr(other.CumulativeSize)
	b.ActiveSize = copyFloat64Ptr(other.ActiveSize)
	b.PriceDecimalPlaces = other.PriceDecimalPlaces
	b.SizeDecimalPlaces = other.SizeDecimalPlaces
}

// CopyEssentials copies only the fast-path fields: Price, Size, IsBid,
// ActiveSize, and CumulativeSize.
func (b *BookItem) CopyEssentials(other *BookItem) {
	b.Price = copyFloat64Ptr(other.Price)
	b.Size = copyFloat64Ptr(other.Size)
	b.IsBid = other.IsBid
	b.ActiveSize = copyFloat64Ptr(other.ActiveSize)
	b.CumulativeSize = copyFloat64Ptr(other.CumulativeSize)
}

// Reset zeroes out all fields to their default values.
func (b *BookItem) Reset() {
	*b = BookItem{}
}

// Equals compares two BookItems by IsBid, EntryID, Price, and Size.
func (b *BookItem) Equals(other *BookItem) bool {
	if other == nil {
		return false
	}
	if b.IsBid != other.IsBid {
		return false
	}
	if b.EntryID != other.EntryID {
		return false
	}
	if !float64PtrEqual(b.Price, other.Price) {
		return false
	}
	if !float64PtrEqual(b.Size, other.Size) {
		return false
	}
	return true
}

// FormattedPrice formats the price using PriceDecimalPlaces.
func (b *BookItem) FormattedPrice() string {
	if b.Price == nil {
		return ""
	}
	return fmt.Sprintf("%.*f", b.PriceDecimalPlaces, *b.Price)
}

// FormattedSize formats the size using kilo/mega suffixes.
// Values >= 1,000,000 are formatted as "XM", values >= 1,000 as "XK".
func (b *BookItem) FormattedSize() string {
	if b.Size == nil {
		return ""
	}
	v := *b.Size
	abs := math.Abs(v)
	switch {
	case abs >= 1_000_000:
		return formatSuffix(v, 1_000_000, "M")
	case abs >= 1_000:
		return formatSuffix(v, 1_000, "K")
	default:
		return fmt.Sprintf("%g", v)
	}
}

// formatSuffix divides v by divisor and appends the suffix, trimming
// unnecessary trailing zeros.
func formatSuffix(v, divisor float64, suffix string) string {
	scaled := v / divisor
	// Use up to 1 decimal place; trim ".0" if it rounds evenly.
	s := fmt.Sprintf("%.1f", scaled)
	// Trim trailing ".0"
	if len(s) >= 2 && s[len(s)-2:] == ".0" {
		s = s[:len(s)-2]
	}
	return s + suffix
}

// copyFloat64Ptr returns a deep copy of a *float64 (nil-safe).
func copyFloat64Ptr(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// float64PtrEqual compares two *float64 values, treating nil == nil.
func float64PtrEqual(a, b *float64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
