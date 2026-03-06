package backfill

import "time"

// TimeRange defines the period to backfill.
type TimeRange struct {
	Start time.Time
	End   time.Time
}

// RawTrade is a Binance aggregate trade from the REST API.
type RawTrade struct {
	ID      int64  `json:"a"`
	Price   string `json:"p"`
	Qty     string `json:"q"`
	First   int64  `json:"f"`
	Last    int64  `json:"l"`
	Time    int64  `json:"T"`
	IsMaker bool   `json:"m"` // true = seller is maker (buyer is taker)
}

// RawFunding is a Binance funding rate record.
type RawFunding struct {
	Symbol      string `json:"symbol"`
	FundingRate string `json:"fundingRate"`
	FundingTime int64  `json:"fundingTime"`
}

// RawOI is a Binance open interest history record.
type RawOI struct {
	Symbol               string `json:"symbol"`
	SumOpenInterest      string `json:"sumOpenInterest"`
	SumOpenInterestValue string `json:"sumOpenInterestValue"`
	Timestamp            int64  `json:"timestamp"`
}

// Checkpoint stores progress for resumable backfill.
type Checkpoint struct {
	Symbol    string    `json:"symbol"`
	DataType  string    `json:"dataType"`
	LastTS    int64     `json:"lastTs"`
	UpdatedAt time.Time `json:"updatedAt"`
}
