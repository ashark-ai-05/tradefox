package kiyotaka

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// Client is an HTTP client for the Kiyotaka REST API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewClient creates a Kiyotaka API client.
func NewClient(cfg Config, logger *slog.Logger) *Client {
	base := cfg.BaseURL
	if base == "" {
		base = "https://api.kiyotaka.ai"
	}
	return &Client{
		baseURL: base,
		apiKey:  cfg.APIKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

// get performs an authenticated GET request.
func (c *Client) get(ctx context.Context, path string, params url.Values) ([]byte, error) {
	u := fmt.Sprintf("%s%s", c.baseURL, path)
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("kiyotaka: build request: %w", err)
	}
	req.Header.Set("x-kiyotaka-key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kiyotaka: request %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("kiyotaka: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kiyotaka: %s returned %d: %s", path, resp.StatusCode, string(body))
	}

	return body, nil
}

// FetchOI fetches open interest for a symbol.
func (c *Client) FetchOI(ctx context.Context, exchange, pair, category string) (*OIDataPoint, error) {
	params := url.Values{
		"exchange": {exchange},
		"symbol":   {pair},
		"category": {category},
	}

	body, err := c.get(ctx, "/v1/open-interest", params)
	if err != nil {
		return nil, err
	}

	var resp APIResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kiyotaka: decode OI: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, nil
	}

	d := resp.Data[0]
	val, _ := toFloat64(d["openInterest"])
	ts, _ := toTimestamp(d["timestamp"])

	return &OIDataPoint{
		Timestamp: ts,
		Symbol:    pair,
		Exchange:  exchange,
		Value:     val,
	}, nil
}

// FetchFundingRate fetches the current funding rate for a symbol.
func (c *Client) FetchFundingRate(ctx context.Context, exchange, pair, category string) (*FundingDataPoint, error) {
	params := url.Values{
		"exchange": {exchange},
		"symbol":   {pair},
		"category": {category},
	}

	body, err := c.get(ctx, "/v1/funding-rate", params)
	if err != nil {
		return nil, err
	}

	var resp APIResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kiyotaka: decode funding: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, nil
	}

	d := resp.Data[0]
	rate, _ := toFloat64(d["fundingRate"])
	ts, _ := toTimestamp(d["timestamp"])

	return &FundingDataPoint{
		Timestamp: ts,
		Symbol:    pair,
		Exchange:  exchange,
		Rate:      rate,
	}, nil
}

// FetchLiquidations fetches recent liquidation data.
func (c *Client) FetchLiquidations(ctx context.Context, exchange, pair, category string) (*LiquidationDataPoint, error) {
	params := url.Values{
		"exchange": {exchange},
		"symbol":   {pair},
		"category": {category},
	}

	body, err := c.get(ctx, "/v1/liquidations", params)
	if err != nil {
		return nil, err
	}

	var resp APIResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kiyotaka: decode liquidations: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, nil
	}

	d := resp.Data[0]
	buyVol, _ := toFloat64(d["buyVolume"])
	sellVol, _ := toFloat64(d["sellVolume"])
	ts, _ := toTimestamp(d["timestamp"])

	return &LiquidationDataPoint{
		Timestamp:  ts,
		Symbol:     pair,
		Exchange:   exchange,
		BuyVolume:  buyVol,
		SellVolume: sellVol,
	}, nil
}

// FetchOHLCV fetches candle data for a symbol.
func (c *Client) FetchOHLCV(ctx context.Context, exchange, pair, category, interval string) (*OHLCVDataPoint, error) {
	params := url.Values{
		"exchange": {exchange},
		"symbol":   {pair},
		"category": {category},
		"interval": {interval},
	}

	body, err := c.get(ctx, "/v1/ohlcv", params)
	if err != nil {
		return nil, err
	}

	var resp APIResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kiyotaka: decode ohlcv: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, nil
	}

	d := resp.Data[0]
	o, _ := toFloat64(d["open"])
	h, _ := toFloat64(d["high"])
	l, _ := toFloat64(d["low"])
	cl, _ := toFloat64(d["close"])
	v, _ := toFloat64(d["volume"])
	ts, _ := toTimestamp(d["timestamp"])

	return &OHLCVDataPoint{
		Timestamp: ts,
		Symbol:    pair,
		Exchange:  exchange,
		Interval:  interval,
		Open:      o,
		High:      h,
		Low:       l,
		Close:     cl,
		Volume:    v,
	}, nil
}

// helpers

func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func toTimestamp(v interface{}) (time.Time, bool) {
	switch n := v.(type) {
	case float64:
		return time.UnixMilli(int64(n)), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return time.Time{}, false
		}
		return time.UnixMilli(i), true
	case string:
		t, err := time.Parse(time.RFC3339, n)
		return t, err == nil
	default:
		return time.Time{}, false
	}
}
