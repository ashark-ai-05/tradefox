package recorder

// OrderBookRecord is the on-disk format for order book snapshots.
// Field names are kept short for compact JSONL output.
type OrderBookRecord struct {
	Type       string        `json:"type"`
	Symbol     string        `json:"symbol"`
	Provider   string        `json:"provider"`
	Sequence   int64         `json:"seq"`
	ExchangeTS int64         `json:"exchange_ts"`
	LocalTS    int64         `json:"local_ts"`
	MidPrice   float64       `json:"mid"`
	Spread     float64       `json:"spread"`
	MicroPrice float64       `json:"micro"`
	Bids       []LevelRecord `json:"bids"`
	Asks       []LevelRecord `json:"asks"`
}

// LevelRecord is a single price level in an OrderBookRecord.
type LevelRecord struct {
	Price float64 `json:"p"`
	Size  float64 `json:"s"`
}

// TradeRecord is the on-disk format for individual trades.
type TradeRecord struct {
	Type       string  `json:"type"`
	Symbol     string  `json:"symbol"`
	Provider   string  `json:"provider"`
	Price      string  `json:"price"`
	Size       string  `json:"size"`
	ExchangeTS int64   `json:"exchange_ts"`
	LocalTS    int64   `json:"local_ts"`
	IsBuy      *bool   `json:"is_buy"`
	MidPrice   float64 `json:"mid"`
}

// KiyotakaRecord is the on-disk format for Kiyotaka study data.
type KiyotakaRecord struct {
	Type      string  `json:"type"`
	Symbol    string  `json:"symbol"`
	Exchange  string  `json:"exchange"`
	Timestamp int64   `json:"ts"`
	LocalTS   int64   `json:"local_ts"`
	Value     float64 `json:"value,omitempty"`
	Open      float64 `json:"o,omitempty"`
	High      float64 `json:"h,omitempty"`
	Low       float64 `json:"l,omitempty"`
	Close     float64 `json:"c,omitempty"`
	Volume    float64 `json:"v,omitempty"`
	Rate      float64 `json:"rate,omitempty"`
	Side      string  `json:"side,omitempty"`
}
