package kiyotaka

import "time"

// Config holds Kiyotaka API configuration.
type Config struct {
	APIKey  string         `json:"apiKey"`
	BaseURL string         `json:"baseUrl"`
	Enabled bool           `json:"enabled"`
	Symbols []SymbolConfig `json:"symbols"`
}

// SymbolConfig maps a trading pair to its Kiyotaka identifiers.
type SymbolConfig struct {
	Symbol   string `json:"symbol"`
	Exchange string `json:"exchange"`
	Pair     string `json:"pair"`
	Category string `json:"category"`
}

// OIDataPoint represents an open interest data point.
type OIDataPoint struct {
	Timestamp time.Time
	Symbol    string
	Exchange  string
	Value     float64
}

// FundingDataPoint represents a funding rate data point.
type FundingDataPoint struct {
	Timestamp time.Time
	Symbol    string
	Exchange  string
	Rate      float64
}

// LiquidationDataPoint represents aggregated liquidation data.
type LiquidationDataPoint struct {
	Timestamp  time.Time
	Symbol     string
	Exchange   string
	BuyVolume  float64
	SellVolume float64
}

// OHLCVDataPoint represents a candle.
type OHLCVDataPoint struct {
	Timestamp time.Time
	Symbol    string
	Exchange  string
	Interval  string
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
}

// APIResponse is the generic envelope from the Kiyotaka REST API.
type APIResponse struct {
	Data []map[string]interface{} `json:"data"`
}
