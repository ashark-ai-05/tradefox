package live

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"
)

// WatchlistTicker holds a single ticker result from the Binance REST API.
type WatchlistTicker struct {
	Symbol   string
	Price    float64
	Change24 float64
	Volume   float64
}

// FetchWatchlistTickers calls the Binance Futures public REST API for 24hr tickers.
// No API key needed.
func FetchWatchlistTickers(ctx context.Context) ([]WatchlistTicker, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://fapi.binance.com/fapi/v1/ticker/24hr", nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var raw []struct {
		Symbol      string `json:"symbol"`
		LastPrice   string `json:"lastPrice"`
		PriceChange string `json:"priceChangePercent"`
		Volume      string `json:"volume"`
		QuoteVolume string `json:"quoteVolume"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	tickers := make([]WatchlistTicker, 0, len(raw))
	for _, r := range raw {
		price, _ := strconv.ParseFloat(r.LastPrice, 64)
		change, _ := strconv.ParseFloat(r.PriceChange, 64)
		vol, _ := strconv.ParseFloat(r.QuoteVolume, 64)
		tickers = append(tickers, WatchlistTicker{
			Symbol:   r.Symbol,
			Price:    price,
			Change24: change,
			Volume:   vol,
		})
	}

	return tickers, nil
}
