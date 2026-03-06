package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// FetchOI fetches the current open interest for a symbol.
func FetchOI(ctx context.Context, baseURL, symbol string) (float64, error) {
	url := fmt.Sprintf("%s/fapi/v1/openInterest?symbol=%s", baseURL, symbol)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		OpenInterest string `json:"openInterest"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decode OI: %w", err)
	}

	oi, _ := strconv.ParseFloat(result.OpenInterest, 64)
	return oi, nil
}

// FetchOIHistory fetches historical open interest data.
func FetchOIHistory(ctx context.Context, baseURL, symbol, period string, limit int) ([]OIPoint, error) {
	url := fmt.Sprintf("%s/futures/data/openInterestHist?symbol=%s&period=%s&limit=%d", baseURL, symbol, period, limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var raw []struct {
		SumOpenInterest string `json:"sumOpenInterest"`
		Timestamp       int64  `json:"timestamp"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode OI history: %w", err)
	}

	points := make([]OIPoint, len(raw))
	for i, r := range raw {
		oi, _ := strconv.ParseFloat(r.SumOpenInterest, 64)
		points[i] = OIPoint{OI: oi, Timestamp: r.Timestamp}
	}
	return points, nil
}

// CalcOIChange computes OI change percentages across timeframes.
func CalcOIChange(history []OIPoint, currentOI float64) OIChange {
	if len(history) == 0 || currentOI == 0 {
		return OIChange{RawOI: currentOI, State: "Stable"}
	}

	now := history[len(history)-1].Timestamp

	var change1H, change4H, change24H float64

	// Find OI values at approximate time offsets
	for _, p := range history {
		age := now - p.Timestamp
		if age >= 3500000 && age <= 3700000 && p.OI > 0 { // ~1h in ms
			change1H = (currentOI - p.OI) / p.OI * 100
		}
		if age >= 14300000 && age <= 14500000 && p.OI > 0 { // ~4h in ms
			change4H = (currentOI - p.OI) / p.OI * 100
		}
		if age >= 86000000 && age <= 86800000 && p.OI > 0 { // ~24h in ms
			change24H = (currentOI - p.OI) / p.OI * 100
		}
	}

	// If exact matches weren't found, use first/last available
	if change1H == 0 && len(history) >= 2 {
		last := history[len(history)-1]
		if last.OI > 0 {
			change1H = (currentOI - last.OI) / last.OI * 100
		}
	}
	if change4H == 0 && len(history) >= 5 {
		idx := len(history) - 5
		if history[idx].OI > 0 {
			change4H = (currentOI - history[idx].OI) / history[idx].OI * 100
		}
	}
	if change24H == 0 && len(history) > 0 {
		first := history[0]
		if first.OI > 0 {
			change24H = (currentOI - first.OI) / first.OI * 100
		}
	}

	state := "Stable"
	if change4H > 5 {
		state = "Building"
	} else if change4H < -5 {
		state = "Declining"
	}

	return OIChange{
		Change1H:  change1H,
		Change4H:  change4H,
		Change24H: change24H,
		RawOI:     currentOI,
		State:     state,
	}
}
