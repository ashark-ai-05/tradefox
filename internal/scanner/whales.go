package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// DefaultWhaleThreshold is the minimum notional value (USD) to consider a trade a whale trade.
const DefaultWhaleThreshold = 500000.0

// FetchRecentTrades fetches recent aggregated trades from Binance futures.
func FetchRecentTrades(ctx context.Context, baseURL, symbol string, limit int) ([]AggTrade, error) {
	url := fmt.Sprintf("%s/fapi/v1/aggTrades?symbol=%s&limit=%d", baseURL, symbol, limit)

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
		Price        string `json:"p"`
		Quantity     string `json:"q"`
		IsBuyerMaker bool   `json:"m"`
		Timestamp    int64  `json:"T"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode agg trades: %w", err)
	}

	trades := make([]AggTrade, len(raw))
	for i, r := range raw {
		price, _ := strconv.ParseFloat(r.Price, 64)
		qty, _ := strconv.ParseFloat(r.Quantity, 64)
		trades[i] = AggTrade{
			Price:        price,
			Qty:          qty,
			IsBuyerMaker: r.IsBuyerMaker,
			Time:         r.Timestamp,
		}
	}
	return trades, nil
}

// DetectWhales filters trades exceeding the notional threshold.
func DetectWhales(trades []AggTrade, threshold float64) []WhaleTrade {
	if threshold <= 0 {
		threshold = DefaultWhaleThreshold
	}

	var whales []WhaleTrade
	for _, t := range trades {
		notional := t.Price * t.Qty
		if notional >= threshold {
			side := "Buy"
			if t.IsBuyerMaker {
				side = "Sell"
			}
			whales = append(whales, WhaleTrade{
				Price:    t.Price,
				Size:     t.Qty,
				Notional: notional,
				Side:     side,
				Time:     t.Time,
			})
		}
	}
	return whales
}

// SummarizeWhales produces a summary of whale trades.
func SummarizeWhales(whales []WhaleTrade) WhaleSummary {
	if len(whales) == 0 {
		return WhaleSummary{NetSide: "Neutral"}
	}

	var totalNotional, largest float64
	var buyNotional, sellNotional float64
	var lastSeen int64

	for _, w := range whales {
		totalNotional += w.Notional
		if w.Notional > largest {
			largest = w.Notional
		}
		if w.Time > lastSeen {
			lastSeen = w.Time
		}
		if w.Side == "Buy" {
			buyNotional += w.Notional
		} else {
			sellNotional += w.Notional
		}
	}

	netSide := "Neutral"
	if buyNotional > sellNotional*1.2 {
		netSide = "Buy"
	} else if sellNotional > buyNotional*1.2 {
		netSide = "Sell"
	}

	return WhaleSummary{
		Count:         len(whales),
		TotalNotional: totalNotional,
		NetSide:       netSide,
		LargestTrade:  largest,
		LastSeen:      lastSeen,
	}
}
