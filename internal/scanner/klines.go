package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// KlineCache caches kline data by symbol+interval with TTL-based invalidation.
type KlineCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
}

type cacheEntry struct {
	candles   []Candle
	fetchedAt time.Time
	ttl       time.Duration
}

// NewKlineCache creates a new LRU-style kline cache.
func NewKlineCache() *KlineCache {
	return &KlineCache{
		entries: make(map[string]*cacheEntry),
	}
}

func (c *KlineCache) key(symbol, interval string) string {
	return symbol + ":" + interval
}

func (c *KlineCache) get(symbol, interval string) ([]Candle, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	e, ok := c.entries[c.key(symbol, interval)]
	if !ok {
		return nil, false
	}
	if time.Since(e.fetchedAt) > e.ttl {
		return nil, false
	}
	return e.candles, true
}

func (c *KlineCache) set(symbol, interval string, candles []Candle, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[c.key(symbol, interval)] = &cacheEntry{
		candles:   candles,
		fetchedAt: time.Now(),
		ttl:       ttl,
	}
}

// intervalTTL returns how long to cache klines for a given interval.
func intervalTTL(interval string) time.Duration {
	switch interval {
	case "5m":
		return 2 * time.Minute
	case "15m":
		return 5 * time.Minute
	case "1h":
		return 15 * time.Minute
	case "4h":
		return 30 * time.Minute
	case "12h":
		return 1 * time.Hour
	case "1d":
		return 2 * time.Hour
	case "1w":
		return 6 * time.Hour
	case "1M":
		return 12 * time.Hour
	default:
		return 5 * time.Minute
	}
}

// BaseURL returns the appropriate base URL for the given market type.
func BaseURL(market string) string {
	if market == "spot" {
		return "https://api.binance.com"
	}
	return "https://fapi.binance.com"
}

// KlinePath returns the API path for klines based on market type.
func KlinePath(market string) string {
	if market == "spot" {
		return "/api/v3/klines"
	}
	return "/fapi/v1/klines"
}

// TickerPath returns the API path for 24h ticker based on market type.
func TickerPath(market string) string {
	if market == "spot" {
		return "/api/v3/ticker/24hr"
	}
	return "/fapi/v1/ticker/24hr"
}

// FetchKlines fetches kline data from Binance REST API.
func FetchKlines(ctx context.Context, baseURL, klinePath, symbol, interval string, limit int) ([]Candle, error) {
	url := fmt.Sprintf("%s%s?symbol=%s&interval=%s&limit=%d", baseURL, klinePath, symbol, interval, limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("rate limited (429)")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var raw [][]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode klines: %w", err)
	}

	candles := make([]Candle, 0, len(raw))
	for _, k := range raw {
		if len(k) < 11 {
			continue
		}
		c, err := parseKline(k)
		if err != nil {
			continue
		}
		candles = append(candles, c)
	}

	return candles, nil
}

func parseKline(k []json.RawMessage) (Candle, error) {
	var openTime int64
	var openS, highS, lowS, closeS, volS string
	var closeTime int64

	if err := json.Unmarshal(k[0], &openTime); err != nil {
		return Candle{}, err
	}
	if err := json.Unmarshal(k[1], &openS); err != nil {
		return Candle{}, err
	}
	if err := json.Unmarshal(k[2], &highS); err != nil {
		return Candle{}, err
	}
	if err := json.Unmarshal(k[3], &lowS); err != nil {
		return Candle{}, err
	}
	if err := json.Unmarshal(k[4], &closeS); err != nil {
		return Candle{}, err
	}
	if err := json.Unmarshal(k[5], &volS); err != nil {
		return Candle{}, err
	}
	if err := json.Unmarshal(k[6], &closeTime); err != nil {
		return Candle{}, err
	}

	open, _ := strconv.ParseFloat(openS, 64)
	high, _ := strconv.ParseFloat(highS, 64)
	low, _ := strconv.ParseFloat(lowS, 64)
	cl, _ := strconv.ParseFloat(closeS, 64)
	vol, _ := strconv.ParseFloat(volS, 64)

	return Candle{
		Open:      open,
		High:      high,
		Low:       low,
		Close:     cl,
		Volume:    vol,
		OpenTime:  openTime,
		CloseTime: closeTime,
	}, nil
}

// Fetch24hChange fetches the 24h price change percentage for a symbol.
func Fetch24hChange(ctx context.Context, baseURL, tickerPath, symbol string) (float64, float64, error) {
	url := fmt.Sprintf("%s%s?symbol=%s", baseURL, tickerPath, symbol)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, 0, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var ticker struct {
		PriceChangePercent string `json:"priceChangePercent"`
		LastPrice          string `json:"lastPrice"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ticker); err != nil {
		return 0, 0, err
	}

	changePct, _ := strconv.ParseFloat(ticker.PriceChangePercent, 64)
	lastPrice, _ := strconv.ParseFloat(ticker.LastPrice, 64)
	return changePct, lastPrice, nil
}
