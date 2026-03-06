package backfill

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"golang.org/x/time/rate"
)

const (
	binanceFuturesBase = "https://fapi.binance.com"
	maxTradesPerReq    = 1000
	maxKlinesPerReq    = 1500
	maxFundingPerReq   = 1000
	maxOIPerReq        = 500
)

// Client is a rate-limited Binance Futures REST API client.
type Client struct {
	http    *http.Client
	baseURL string
	limiter *rate.Limiter
	logger  *slog.Logger
}

// NewClient creates a Binance Futures client with rate limiting.
// Binance IP limit is 2400 weight/min. We use 600/min (~10 req/s) for safety,
// since other processes may share the IP and some endpoints carry higher weight.
func NewClient(logger *slog.Logger) *Client {
	return &Client{
		http:    &http.Client{Timeout: 30 * time.Second},
		baseURL: binanceFuturesBase,
		limiter: rate.NewLimiter(rate.Limit(10), 3), // ~10 req/s = 600/min (conservative)
		logger:  logger,
	}
}

// get makes a rate-limited GET request with retry on 429 and unmarshals the response.
func (c *Client) get(ctx context.Context, path string, params url.Values, out any) error {
	const maxRetries = 5

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limit wait: %w", err)
		}

		u := c.baseURL + path
		if len(params) > 0 {
			u += "?" + params.Encode()
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return err
		}

		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			// Exponential backoff: 5s, 15s, 45s, 135s, 405s
			backoff := time.Duration(5) * time.Second
			for i := 0; i < attempt; i++ {
				backoff *= 3
			}
			// Check Retry-After header
			if ra, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && ra > 0 {
				backoff = time.Duration(ra) * time.Second
			}
			c.logger.Warn("rate limited by Binance, backing off",
				slog.String("path", path),
				slog.Duration("backoff", backoff),
				slog.Int("attempt", attempt+1))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				continue
			}
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return fmt.Errorf("binance API %s: status %d: %s", path, resp.StatusCode, string(body))
		}

		err = json.NewDecoder(resp.Body).Decode(out)
		resp.Body.Close()
		return err
	}

	return fmt.Errorf("binance API %s: max retries (%d) exceeded on rate limit", path, maxRetries)
}

// FetchAggTrades fetches aggregate trades for a symbol in [startTime, endTime).
// Returns up to 1000 trades per call.
func (c *Client) FetchAggTrades(ctx context.Context, symbol string, startTime, endTime int64) ([]RawTrade, error) {
	params := url.Values{
		"symbol":    {symbol},
		"startTime": {strconv.FormatInt(startTime, 10)},
		"endTime":   {strconv.FormatInt(endTime, 10)},
		"limit":     {strconv.Itoa(maxTradesPerReq)},
	}
	var trades []RawTrade
	if err := c.get(ctx, "/fapi/v1/aggTrades", params, &trades); err != nil {
		return nil, err
	}
	return trades, nil
}

// FetchKlines fetches 1m klines for a symbol in [startTime, endTime).
// Returns raw JSON arrays that need custom parsing.
func (c *Client) FetchKlines(ctx context.Context, symbol string, startTime, endTime int64) ([]json.RawMessage, error) {
	params := url.Values{
		"symbol":    {symbol},
		"interval":  {"1m"},
		"startTime": {strconv.FormatInt(startTime, 10)},
		"endTime":   {strconv.FormatInt(endTime, 10)},
		"limit":     {strconv.Itoa(maxKlinesPerReq)},
	}
	var klines []json.RawMessage
	if err := c.get(ctx, "/fapi/v1/klines", params, &klines); err != nil {
		return nil, err
	}
	return klines, nil
}

// FetchFundingHistory fetches historical funding rates.
func (c *Client) FetchFundingHistory(ctx context.Context, symbol string, startTime, endTime int64) ([]RawFunding, error) {
	params := url.Values{
		"symbol":    {symbol},
		"startTime": {strconv.FormatInt(startTime, 10)},
		"endTime":   {strconv.FormatInt(endTime, 10)},
		"limit":     {strconv.Itoa(maxFundingPerReq)},
	}
	var funding []RawFunding
	if err := c.get(ctx, "/fapi/v1/fundingRate", params, &funding); err != nil {
		return nil, err
	}
	return funding, nil
}

// FetchOIHistory fetches open interest history at 5-minute intervals.
func (c *Client) FetchOIHistory(ctx context.Context, symbol string, startTime, endTime int64) ([]RawOI, error) {
	params := url.Values{
		"symbol":    {symbol},
		"period":    {"5m"},
		"startTime": {strconv.FormatInt(startTime, 10)},
		"endTime":   {strconv.FormatInt(endTime, 10)},
		"limit":     {strconv.Itoa(maxOIPerReq)},
	}
	var oi []RawOI
	if err := c.get(ctx, "/futures/data/openInterestHist", params, &oi); err != nil {
		return nil, err
	}
	return oi, nil
}
