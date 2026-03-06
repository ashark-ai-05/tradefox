package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ashark-ai-05/tradefox/internal/tui/live"
)

// chartMessage is the JSON envelope for chart WebSocket messages.
type chartMessage struct {
	Type      string      `json:"type"`
	Timeframe string      `json:"timeframe,omitempty"`
	Data      interface{} `json:"data"`
}

type candleData struct {
	Time   int64   `json:"time"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

type tradeData struct {
	Price float64 `json:"price"`
	Size  float64 `json:"size"`
	Side  string  `json:"side"`
	Time  int64   `json:"time"`
}

type orderbookData struct {
	Bids [][2]float64 `json:"bids"`
	Asks [][2]float64 `json:"asks"`
}

type tickerData struct {
	Price     float64 `json:"price"`
	Change24h float64 `json:"change24h"`
	Volume24h float64 `json:"volume24h"`
	Funding   float64 `json:"funding"`
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// ServeChartWS handles the /ws/chart WebSocket endpoint.
// It creates a BinancePublicFeed per connection and streams live data.
func ServeChartWS(logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		if symbol == "" {
			symbol = "BTCUSDT"
		}

		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			logger.Error("chart ws upgrade failed", "error", err)
			return
		}

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		bridge := live.NewLiveDataBridge(nil)
		upperSymbol := symbol

		// Channel to send messages to the client
		sendCh := make(chan []byte, 256)

		// Register callbacks before connecting
		bridge.SubscribeCandles(upperSymbol, func(c live.Candle) {
			msg := chartMessage{
				Type:      "candle",
				Timeframe: "1m",
				Data: candleData{
					Time: c.Time, Open: c.Open, High: c.High,
					Low: c.Low, Close: c.Close, Volume: c.Volume,
				},
			}
			data, err := json.Marshal(msg)
			if err != nil {
				return
			}
			select {
			case sendCh <- data:
			default:
			}
		})

		bridge.SubscribeTrades(upperSymbol, func(t live.TradeEvent) {
			side := "buy"
			if t.Side == "SELL" {
				side = "sell"
			}
			msg := chartMessage{
				Type: "trade",
				Data: tradeData{
					Price: t.Price, Size: t.Size,
					Side: side, Time: t.Time.UnixMilli(),
				},
			}
			data, err := json.Marshal(msg)
			if err != nil {
				return
			}
			select {
			case sendCh <- data:
			default:
			}
		})

		bridge.SubscribeOrderBook(upperSymbol, func(bids, asks []live.OrderBookLevel) {
			ob := orderbookData{
				Bids: make([][2]float64, 0, len(bids)),
				Asks: make([][2]float64, 0, len(asks)),
			}
			for _, b := range bids {
				ob.Bids = append(ob.Bids, [2]float64{b.Price, b.Qty})
			}
			for _, a := range asks {
				ob.Asks = append(ob.Asks, [2]float64{a.Price, a.Qty})
			}
			msg := chartMessage{Type: "orderbook", Data: ob}
			data, err := json.Marshal(msg)
			if err != nil {
				return
			}
			select {
			case sendCh <- data:
			default:
			}
		})

		bridge.SubscribeTicker(upperSymbol, func(t live.TickerUpdate) {
			msg := chartMessage{
				Type: "ticker",
				Data: tickerData{
					Price:     t.Price,
					Change24h: t.Change24,
					Volume24h: t.Volume,
					Funding:   t.FundingRate,
				},
			}
			data, err := json.Marshal(msg)
			if err != nil {
				return
			}
			select {
			case sendCh <- data:
			default:
			}
		})

		// Connect to Binance public feed
		if err := bridge.ConnectPublic(ctx, symbol); err != nil {
			logger.Error("chart ws feed connect failed", "error", err, "symbol", symbol)
			conn.Close()
			return
		}
		defer bridge.Close()

		logger.Info("chart ws client connected", "symbol", symbol)

		// Write pump
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			defer conn.Close()

			for {
				select {
				case <-ctx.Done():
					return
				case data := <-sendCh:
					_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
					if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
						cancel()
						return
					}
				case <-ticker.C:
					_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
					if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
						cancel()
						return
					}
				}
			}
		}()

		// Read pump (handles close and subscription messages)
		conn.SetReadLimit(4096)
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		conn.SetPongHandler(func(string) error {
			_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			return nil
		})

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
			_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		}

		logger.Info("chart ws client disconnected", "symbol", symbol)
	}
}

// binanceKline matches the Binance REST API kline response array format.
type binanceKline struct {
	OpenTime  int64
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
	CloseTime int64
}

// GetCandles handles GET /api/candles?symbol=BTCUSDT&timeframe=1h&limit=500
// It proxies to the Binance Futures public REST API.
func GetCandles(logger *slog.Logger) http.HandlerFunc {
	client := &http.Client{Timeout: 10 * time.Second}

	return func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		if symbol == "" {
			symbol = "BTCUSDT"
		}
		timeframe := r.URL.Query().Get("timeframe")
		if timeframe == "" {
			timeframe = "1m"
		}
		limit := 500
		if l := r.URL.Query().Get("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 1500 {
				limit = v
			}
		}

		url := fmt.Sprintf("https://fapi.binance.com/fapi/v1/klines?symbol=%s&interval=%s&limit=%d",
			symbol, timeframe, limit)

		resp, err := client.Get(url)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, errorResponse{Error: "binance api error: " + err.Error()})
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, errorResponse{Error: "read error"})
			return
		}

		if resp.StatusCode != http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(resp.StatusCode)
			_, _ = w.Write(body)
			return
		}

		// Parse Binance kline array format: [[openTime, open, high, low, close, volume, ...], ...]
		var raw [][]json.RawMessage
		if err := json.Unmarshal(body, &raw); err != nil {
			writeJSON(w, http.StatusBadGateway, errorResponse{Error: "parse error"})
			return
		}

		candles := make([]candleData, 0, len(raw))
		for _, k := range raw {
			if len(k) < 6 {
				continue
			}
			var openTime int64
			var open, high, low, close, volume string
			json.Unmarshal(k[0], &openTime)
			json.Unmarshal(k[1], &open)
			json.Unmarshal(k[2], &high)
			json.Unmarshal(k[3], &low)
			json.Unmarshal(k[4], &close)
			json.Unmarshal(k[5], &volume)

			o, _ := strconv.ParseFloat(open, 64)
			h, _ := strconv.ParseFloat(high, 64)
			l, _ := strconv.ParseFloat(low, 64)
			c, _ := strconv.ParseFloat(close, 64)
			v, _ := strconv.ParseFloat(volume, 64)

			candles = append(candles, candleData{
				Time:   openTime / 1000, // convert ms to seconds for lightweight-charts
				Open:   o,
				High:   h,
				Low:    l,
				Close:  c,
				Volume: v,
			})
		}

		writeJSON(w, http.StatusOK, candles)
	}
}
