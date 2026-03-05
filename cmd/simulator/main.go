// Demo Trading Simulator for VisualHFT.
//
// Generates fake market data (order book snapshots, trades, heartbeats) and
// sends them to the VisualHFT server via the Generic WebSocket connector
// format. Useful for development, demos, and testing without connecting to
// live exchanges.
//
// Usage:
//
//	go run ./cmd/simulator --addr localhost:8080 --symbols BTC/USD,ETH/USD
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

type config struct {
	addr        string
	symbols     string
	intervalMs  int
	initialBid  float64
	volatility  float64
	providerID  int
	providerName string
	depthLevels int
}

func parseFlags() config {
	c := config{}
	flag.StringVar(&c.addr, "addr", "localhost:8080", "WebSocket server address (host:port)")
	flag.StringVar(&c.symbols, "symbols", "BTC/USD,ETH/USD", "Comma-separated list of symbols")
	flag.IntVar(&c.intervalMs, "interval", 200, "Update interval in milliseconds")
	flag.Float64Var(&c.initialBid, "price", 30000.0, "Initial bid price")
	flag.Float64Var(&c.volatility, "volatility", 0.0005, "Price volatility (fraction per tick)")
	flag.IntVar(&c.providerID, "provider-id", 99, "Provider ID")
	flag.StringVar(&c.providerName, "provider-name", "DemoSimulator", "Provider name")
	flag.IntVar(&c.depthLevels, "depth", 10, "Order book depth levels per side")
	flag.Parse()
	return c
}

// ---------------------------------------------------------------------------
// Message types (Generic WebSocket connector format)
// ---------------------------------------------------------------------------

type envelope struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type marketItem struct {
	ProviderID   int     `json:"providerId"`
	ProviderName string  `json:"providerName"`
	Symbol       string  `json:"symbol"`
	IsBid        bool    `json:"isBid"`
	Price        float64 `json:"price"`
	Size         float64 `json:"size"`
	Timestamp    string  `json:"timestamp"`
}

type tradeItem struct {
	ProviderID   int     `json:"providerId"`
	ProviderName string  `json:"providerName"`
	Symbol       string  `json:"symbol"`
	Price        float64 `json:"price"`
	Size         float64 `json:"size"`
	IsBuy        bool    `json:"isBuy"`
	Timestamp    string  `json:"timestamp"`
}

type heartbeatItem struct {
	ProviderID   int    `json:"providerId"`
	ProviderName string `json:"providerName"`
	Timestamp    string `json:"timestamp"`
}

// ---------------------------------------------------------------------------
// Simulator
// ---------------------------------------------------------------------------

type symbolState struct {
	bidPrice float64
}

func main() {
	cfg := parseFlags()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	symbols := strings.Split(cfg.symbols, ",")
	for i := range symbols {
		symbols[i] = strings.TrimSpace(symbols[i])
	}

	// Initialize per-symbol state.
	states := make(map[string]*symbolState, len(symbols))
	for i, sym := range symbols {
		// Give each symbol a different initial price.
		states[sym] = &symbolState{
			bidPrice: cfg.initialBid * (1.0 - float64(i)*0.3),
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Connect to the server's WebSocket endpoint.
	u := url.URL{Scheme: "ws", Host: cfg.addr, Path: "/ws"}
	logger.Info("connecting to server", slog.String("url", u.String()))

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		logger.Error("websocket dial failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer conn.Close()

	logger.Info("connected, starting simulation",
		slog.String("symbols", strings.Join(symbols, ",")),
		slog.Int("intervalMs", cfg.intervalMs),
	)

	ticker := time.NewTicker(time.Duration(cfg.intervalMs) * time.Millisecond)
	defer ticker.Stop()

	heartbeatTicker := time.NewTicker(3 * time.Second)
	defer heartbeatTicker.Stop()

	tickCount := 0
	for {
		select {
		case <-ctx.Done():
			logger.Info("shutting down simulator")
			return

		case <-heartbeatTicker.C:
			hb := envelope{
				Type: "HeartBeats",
				Data: heartbeatItem{
					ProviderID:   cfg.providerID,
					ProviderName: cfg.providerName,
					Timestamp:    time.Now().Format(time.RFC3339Nano),
				},
			}
			if err := writeJSON(conn, hb); err != nil {
				logger.Error("write heartbeat failed", slog.Any("error", err))
				return
			}

		case <-ticker.C:
			tickCount++
			now := time.Now()

			for _, sym := range symbols {
				st := states[sym]

				// Random walk for price.
				change := st.bidPrice * cfg.volatility * (rand.Float64()*2 - 1)
				st.bidPrice += change
				if st.bidPrice <= 0 {
					st.bidPrice = math.Abs(st.bidPrice) + 0.01
				}

				spread := st.bidPrice * 0.0001 * (1 + rand.Float64())
				askPrice := st.bidPrice + spread

				// Generate order book levels.
				var levels []marketItem
				for i := 0; i < cfg.depthLevels; i++ {
					bidP := st.bidPrice - float64(i)*spread*0.5
					askP := askPrice + float64(i)*spread*0.5
					bidS := 0.1 + rand.Float64()*5.0
					askS := 0.1 + rand.Float64()*5.0

					levels = append(levels, marketItem{
						ProviderID:   cfg.providerID,
						ProviderName: cfg.providerName,
						Symbol:       sym,
						IsBid:        true,
						Price:        math.Round(bidP*100) / 100,
						Size:         math.Round(bidS*1000) / 1000,
						Timestamp:    now.Format(time.RFC3339Nano),
					})
					levels = append(levels, marketItem{
						ProviderID:   cfg.providerID,
						ProviderName: cfg.providerName,
						Symbol:       sym,
						IsBid:        false,
						Price:        math.Round(askP*100) / 100,
						Size:         math.Round(askS*1000) / 1000,
						Timestamp:    now.Format(time.RFC3339Nano),
					})
				}

				if err := writeJSON(conn, envelope{Type: "Market", Data: levels}); err != nil {
					logger.Error("write market data failed", slog.Any("error", err))
					return
				}

				// Generate trades periodically (roughly every 3rd tick per symbol).
				if tickCount%3 == 0 {
					isBuy := rand.Float64() > 0.5
					tradePrice := st.bidPrice
					if isBuy {
						tradePrice = askPrice
					}
					trade := tradeItem{
						ProviderID:   cfg.providerID,
						ProviderName: cfg.providerName,
						Symbol:       sym,
						Price:        math.Round(tradePrice*100) / 100,
						Size:         math.Round((0.01+rand.Float64()*2.0)*1000) / 1000,
						IsBuy:        isBuy,
						Timestamp:    now.Format(time.RFC3339Nano),
					}
					if err := writeJSON(conn, envelope{Type: "Trades", Data: trade}); err != nil {
						logger.Error("write trade failed", slog.Any("error", err))
						return
					}
				}
			}

			if tickCount%50 == 0 {
				logger.Info("simulation running",
					slog.Int("ticks", tickCount),
					slog.String("prices", formatPrices(symbols, states)),
				)
			}
		}
	}
}

func writeJSON(conn *websocket.Conn, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

func formatPrices(symbols []string, states map[string]*symbolState) string {
	var parts []string
	for _, sym := range symbols {
		parts = append(parts, fmt.Sprintf("%s=%.2f", sym, states[sym].bidPrice))
	}
	return strings.Join(parts, " ")
}
