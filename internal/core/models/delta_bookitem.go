package models

import "time"

// DeltaBookItem represents an incremental update to a single price level
// in the order book. Pointer fields are nullable to distinguish between
// "not provided" and zero values in partial updates.
type DeltaBookItem struct {
	IsBid          *bool     `json:"isBid"`
	EntryID        string    `json:"entryId"`
	Price          *float64  `json:"price"`
	Size           *float64  `json:"size"`
	LocalTimestamp  time.Time `json:"localTimestamp"`
	ServerTimestamp time.Time `json:"serverTimestamp"`
}

// boolVal returns the value of IsBid or false if nil.
func (d *DeltaBookItem) boolVal() bool {
	if d.IsBid == nil {
		return false
	}
	return *d.IsBid
}

// priceVal returns the value of Price or 0 if nil.
func (d *DeltaBookItem) priceVal() float64 {
	if d.Price == nil {
		return 0
	}
	return *d.Price
}

// sizeVal returns the value of Size or 0 if nil.
func (d *DeltaBookItem) sizeVal() float64 {
	if d.Size == nil {
		return 0
	}
	return *d.Size
}
