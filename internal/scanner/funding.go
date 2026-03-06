package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
)

// FetchFundingRate fetches the current funding rate from Binance premium index.
func FetchFundingRate(ctx context.Context, baseURL, symbol string) (FundingData, error) {
	url := fmt.Sprintf("%s/fapi/v1/premiumIndex?symbol=%s", baseURL, symbol)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return FundingData{}, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return FundingData{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return FundingData{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var raw struct {
		LastFundingRate          string `json:"lastFundingRate"`
		NextFundingTime         int64  `json:"nextFundingTime"`
		InterestRate            string `json:"interestRate"`
		EstimatedSettlePrice    string `json:"estimatedSettlePrice"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return FundingData{}, fmt.Errorf("decode funding: %w", err)
	}

	rate, _ := strconv.ParseFloat(raw.LastFundingRate, 64)
	// Predicted is approximated from current rate
	predicted := rate

	state := ClassifyFunding(rate)

	return FundingData{
		Rate:      rate,
		Predicted: predicted,
		NextTime:  raw.NextFundingTime,
		State:     state,
	}, nil
}

// ClassifyFunding classifies a funding rate into a state category.
func ClassifyFunding(rate float64) string {
	absRate := math.Abs(rate)
	negative := rate < 0

	switch {
	case absRate > 0.0005: // > 0.05%
		if negative {
			return "NegExtreme"
		}
		return "Extreme"
	case absRate > 0.0003: // > 0.03%
		if negative {
			return "NegHigh"
		}
		return "High"
	case absRate > 0.0001: // > 0.01%
		if negative {
			return "NegElevated"
		}
		return "Elevated"
	default:
		return "Normal"
	}
}
