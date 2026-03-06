// Package handlers provides HTTP handler functions for the VisualHFT REST API.
// It includes an in-memory DataStore that subscribes to the event bus and
// maintains the latest state for REST queries.
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"

	"github.com/ashark-ai-05/tradefox/internal/config"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
	"github.com/ashark-ai-05/tradefox/internal/plugin"
)

// DataStore holds latest state from the event bus for REST API queries.
// Each field is a sync.Map keyed by the relevant identifier.
type DataStore struct {
	orderbooks sync.Map // symbol → *models.OrderBook
	trades     sync.Map // symbol → *TradeBuffer (ring buffer of recent trades)
	providers  sync.Map // providerID (int) → models.Provider
	studies    sync.Map // studyName (string) → models.BaseStudyModel
	positions  sync.Map // symbol → models.Order (latest position orders)
}

// TradeBuffer is a fixed-size ring buffer for recent trades.
type TradeBuffer struct {
	mu     sync.RWMutex
	trades []models.Trade
	maxLen int
}

// NewTradeBuffer creates a new TradeBuffer with the given maximum length.
func NewTradeBuffer(maxLen int) *TradeBuffer {
	if maxLen <= 0 {
		maxLen = 100
	}
	return &TradeBuffer{
		trades: make([]models.Trade, 0, maxLen),
		maxLen: maxLen,
	}
}

// Add appends a trade to the buffer, evicting the oldest trade if full.
func (tb *TradeBuffer) Add(t models.Trade) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if len(tb.trades) >= tb.maxLen {
		// Shift left by one to drop the oldest entry.
		copy(tb.trades, tb.trades[1:])
		tb.trades[len(tb.trades)-1] = t
	} else {
		tb.trades = append(tb.trades, t)
	}
}

// GetAll returns a copy of all trades currently in the buffer.
func (tb *TradeBuffer) GetAll() []models.Trade {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	out := make([]models.Trade, len(tb.trades))
	copy(out, tb.trades)
	return out
}

// NewDataStore creates a new, empty DataStore.
func NewDataStore() *DataStore {
	return &DataStore{}
}

// SubscribeToEventBus subscribes to all event bus topics and updates the
// data store in background goroutines. Each topic is consumed independently.
func (ds *DataStore) SubscribeToEventBus(bus *eventbus.Bus) {
	// OrderBooks
	_, obCh := bus.OrderBooks.Subscribe(64)
	go func() {
		for ob := range obCh {
			if ob == nil {
				continue
			}
			ds.orderbooks.Store(ob.Symbol, ob)
		}
	}()

	// Trades
	_, trCh := bus.Trades.Subscribe(256)
	go func() {
		for t := range trCh {
			val, _ := ds.trades.LoadOrStore(t.Symbol, NewTradeBuffer(500))
			buf := val.(*TradeBuffer)
			buf.Add(t)
		}
	}()

	// Providers
	_, prCh := bus.Providers.Subscribe(32)
	go func() {
		for p := range prCh {
			ds.providers.Store(p.ProviderID, p)
		}
	}()

	// Studies
	_, stCh := bus.Studies.Subscribe(64)
	go func() {
		for s := range stCh {
			// Key by formatted value + timestamp to make it unique;
			// in practice, studies are keyed by a name we derive from the value.
			// Since BaseStudyModel does not carry a name, we key by ValueFormatted
			// or fall back to a composite key. For the REST API, we store the
			// latest study model keyed by ValueFormatted (which acts as study name).
			key := s.ValueFormatted
			if key == "" {
				key = "default"
			}
			ds.studies.Store(key, s)
		}
	}()

	// Positions (Order events)
	_, posCh := bus.Positions.Subscribe(64)
	go func() {
		for o := range posCh {
			ds.positions.Store(o.Symbol, o)
		}
	}()
}

// Deps holds all dependencies for HTTP handlers.
type Deps struct {
	DataStore *DataStore
	PluginMgr *plugin.Manager // can be nil in tests
	Settings  *config.Manager // can be nil in tests
	Logger    *slog.Logger
}

// writeJSON is a helper that encodes data as JSON and writes it to the response
// writer with the given status code and Content-Type: application/json header.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// errorResponse is a standard error payload returned by the API.
type errorResponse struct {
	Error string `json:"error"`
}
