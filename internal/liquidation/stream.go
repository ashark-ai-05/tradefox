package liquidation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// LiquidationEvent represents a real liquidation from the exchange.
type LiquidationEvent struct {
	Symbol    string  `json:"symbol"`
	Side      string  `json:"side"` // "Buy" (short liquidated) or "Sell" (long liquidated)
	Price     float64 `json:"price"`
	Quantity  float64 `json:"quantity"`
	Notional  float64 `json:"notional"`
	Time      int64   `json:"time"`
	OrderType string  `json:"orderType"`
}

// LiquidationFeedStats summarizes recent liquidation activity.
type LiquidationFeedStats struct {
	LongsLiquidated  float64          `json:"longsLiquidated"`  // notional value
	ShortsLiquidated float64          `json:"shortsLiquidated"` // notional value
	Count            int              `json:"count"`
	LargestSingle    LiquidationEvent `json:"largestSingle"`
	Period           time.Duration    `json:"period"`
}

// LiquidationFeed stores recent liquidation events.
type LiquidationFeed struct {
	mu     sync.RWMutex
	events []LiquidationEvent
	maxLen int
}

// NewLiquidationFeed creates a feed that retains the most recent events.
func NewLiquidationFeed() *LiquidationFeed {
	return &LiquidationFeed{
		events: make([]LiquidationEvent, 0, 1000),
		maxLen: 5000,
	}
}

// Add appends a liquidation event.
func (f *LiquidationFeed) Add(event LiquidationEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.events = append(f.events, event)
	if len(f.events) > f.maxLen {
		f.events = f.events[len(f.events)-f.maxLen:]
	}
}

// RecentEvents returns the most recent events for a symbol (or all if symbol is empty).
func (f *LiquidationFeed) RecentEvents(symbol string, limit int) []LiquidationEvent {
	f.mu.RLock()
	defer f.mu.RUnlock()

	var result []LiquidationEvent
	// Iterate backwards for most recent first
	for i := len(f.events) - 1; i >= 0 && len(result) < limit; i-- {
		if symbol == "" || f.events[i].Symbol == symbol {
			result = append(result, f.events[i])
		}
	}
	return result
}

// Stats computes summary statistics for a symbol over a duration.
func (f *LiquidationFeed) Stats(symbol string, duration time.Duration) LiquidationFeedStats {
	cutoff := time.Now().UnixMilli() - duration.Milliseconds()

	f.mu.RLock()
	defer f.mu.RUnlock()

	var stats LiquidationFeedStats
	stats.Period = duration

	for i := len(f.events) - 1; i >= 0; i-- {
		e := f.events[i]
		if e.Time < cutoff {
			break
		}
		if symbol != "" && e.Symbol != symbol {
			continue
		}
		stats.Count++
		// "Sell" force order = long position liquidated
		// "Buy" force order = short position liquidated
		if e.Side == "Sell" {
			stats.LongsLiquidated += e.Notional
		} else {
			stats.ShortsLiquidated += e.Notional
		}
		if e.Notional > stats.LargestSingle.Notional {
			stats.LargestSingle = e
		}
	}
	return stats
}

// binanceForceOrder is the raw JSON shape from Binance's forceOrder stream.
type binanceForceOrder struct {
	E  string `json:"e"` // "forceOrder"
	T  int64  `json:"E"` // event time
	O  struct {
		S    string `json:"s"`  // symbol
		Si   string `json:"S"`  // side (SELL = long liquidated, BUY = short liquidated)
		O    string `json:"o"`  // order type
		F    string `json:"f"`  // time in force
		Q    string `json:"q"`  // quantity
		P    string `json:"p"`  // price
		AP   string `json:"ap"` // average price
		X    string `json:"X"`  // order status
		L    string `json:"l"`  // last filled quantity
		Z    string `json:"z"`  // cumulative filled quantity
		T    int64  `json:"T"`  // trade time
	} `json:"o"`
}

// SubscribeLiquidations connects to Binance's forceOrder WebSocket stream
// and emits parsed LiquidationEvent values on the returned channel.
func SubscribeLiquidations(ctx context.Context, symbol string, logger *slog.Logger) (<-chan LiquidationEvent, error) {
	lowerSymbol := strings.ToLower(symbol)
	url := fmt.Sprintf("wss://fstream.binance.com/ws/%s@forceOrder", lowerSymbol)

	ch := make(chan LiquidationEvent, 64)

	go func() {
		defer close(ch)

		for {
			if err := ctx.Err(); err != nil {
				return
			}

			err := streamForceOrders(ctx, url, ch, logger)
			if err != nil && ctx.Err() == nil {
				logger.Warn("liquidation stream disconnected, reconnecting",
					slog.String("symbol", symbol),
					slog.String("error", err.Error()),
				)
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
				}
			}
		}
	}()

	return ch, nil
}

func streamForceOrders(ctx context.Context, url string, ch chan<- LiquidationEvent, logger *slog.Logger) error {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	// Close connection when context is cancelled
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		var fo binanceForceOrder
		if err := json.Unmarshal(msg, &fo); err != nil {
			logger.Warn("failed to parse force order", slog.String("error", err.Error()))
			continue
		}

		price := parseFloat(fo.O.AP) // use average price
		if price == 0 {
			price = parseFloat(fo.O.P)
		}
		qty := parseFloat(fo.O.Q)

		event := LiquidationEvent{
			Symbol:    fo.O.S,
			Side:      fo.O.Si,
			Price:     price,
			Quantity:  qty,
			Notional:  price * qty,
			Time:      fo.O.T,
			OrderType: fo.O.O,
		}

		select {
		case ch <- event:
		default:
			// Drop if consumer is slow
		}
	}
}

func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}
