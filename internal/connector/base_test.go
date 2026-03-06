package connector

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/core/enums"
	"github.com/ashark-ai-05/tradefox/internal/core/interfaces"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

// newTestConnector creates a BaseConnector wired to a fresh Bus for testing.
func newTestConnector() (*BaseConnector, *eventbus.Bus) {
	logger := slog.Default()
	bus := eventbus.NewBus(logger)
	bc := NewBaseConnector(BaseConnectorConfig{
		Name:         "TestConnector",
		Version:      "1.0.0",
		Description:  "A test connector",
		Author:       "TestAuthor",
		ProviderID:   42,
		ProviderName: "TestProvider",
		Bus:          bus,
		Logger:       logger,
	})
	return bc, bus
}

// Compile-time check: BaseConnector satisfies interfaces.Connector.
var _ interfaces.Connector = (*BaseConnector)(nil)

// ---------------------------------------------------------------------------
// 1. TestBaseConnector_SymbolParsing
// ---------------------------------------------------------------------------

func TestBaseConnector_SymbolParsing(t *testing.T) {
	bc, _ := newTestConnector()

	bc.ParseSymbols("BTCUSDT(BTC/USDT), ETHUSDT")

	// Verify mapping.
	if got := bc.GetNormalizedSymbol("BTCUSDT"); got != "BTC/USDT" {
		t.Errorf("expected BTC/USDT, got %s", got)
	}
	if got := bc.GetNormalizedSymbol("ETHUSDT"); got != "ETHUSDT" {
		t.Errorf("expected ETHUSDT, got %s", got)
	}

	exchangeSyms := bc.GetAllExchangeSymbols()
	sort.Strings(exchangeSyms)
	if len(exchangeSyms) != 2 {
		t.Fatalf("expected 2 exchange symbols, got %d", len(exchangeSyms))
	}
	if exchangeSyms[0] != "BTCUSDT" || exchangeSyms[1] != "ETHUSDT" {
		t.Errorf("unexpected exchange symbols: %v", exchangeSyms)
	}

	normSyms := bc.GetAllNormalizedSymbols()
	sort.Strings(normSyms)
	if len(normSyms) != 2 {
		t.Fatalf("expected 2 normalized symbols, got %d", len(normSyms))
	}
	if normSyms[0] != "BTC/USDT" || normSyms[1] != "ETHUSDT" {
		t.Errorf("unexpected normalized symbols: %v", normSyms)
	}
}

// ---------------------------------------------------------------------------
// 2. TestBaseConnector_GetNormalizedSymbol
// ---------------------------------------------------------------------------

func TestBaseConnector_GetNormalizedSymbol(t *testing.T) {
	bc, _ := newTestConnector()

	// Empty map: should return original.
	if got := bc.GetNormalizedSymbol("FOO"); got != "FOO" {
		t.Errorf("expected FOO (fallback), got %s", got)
	}

	// After parsing, lookup should work.
	bc.ParseSymbols("XRPUSDT(XRP/USD)")

	if got := bc.GetNormalizedSymbol("XRPUSDT"); got != "XRP/USD" {
		t.Errorf("expected XRP/USD, got %s", got)
	}

	// Unknown symbol: fallback.
	if got := bc.GetNormalizedSymbol("MISSING"); got != "MISSING" {
		t.Errorf("expected MISSING (fallback), got %s", got)
	}
}

// ---------------------------------------------------------------------------
// 3. TestBaseConnector_DecimalPlaces
// ---------------------------------------------------------------------------

func TestBaseConnector_DecimalPlaces(t *testing.T) {
	tests := []struct {
		name     string
		values   []float64
		expected int
	}{
		{"two decimals", []float64{1.23, 4.5, 6.0}, 2},
		{"five decimals", []float64{0.12345, 1.0}, 5},
		{"mixed", []float64{100.1, 200.12, 300.123}, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RecognizeDecimalPlaces(tc.values)
			if got != tc.expected {
				t.Errorf("expected %d, got %d", tc.expected, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 4. TestBaseConnector_DecimalPlaces_Minimum1
// ---------------------------------------------------------------------------

func TestBaseConnector_DecimalPlaces_Minimum1(t *testing.T) {
	// All integers: should return minimum 1.
	got := RecognizeDecimalPlaces([]float64{1, 2, 3})
	if got != 1 {
		t.Errorf("expected minimum 1, got %d", got)
	}

	// Empty slice: should also return 1.
	got = RecognizeDecimalPlaces(nil)
	if got != 1 {
		t.Errorf("expected minimum 1 for nil slice, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// 5. TestBaseConnector_Lifecycle
// ---------------------------------------------------------------------------

func TestBaseConnector_Lifecycle(t *testing.T) {
	bc, _ := newTestConnector()
	ctx := context.Background()

	// Initial status should be Loaded.
	if bc.Status() != enums.PluginLoaded {
		t.Errorf("expected PluginLoaded, got %v", bc.Status())
	}

	// Start sets status to Starting.
	if err := bc.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync error: %v", err)
	}
	if bc.Status() != enums.PluginStarting {
		t.Errorf("expected PluginStarting, got %v", bc.Status())
	}

	// Stop sets status to Stopped.
	if err := bc.StopAsync(ctx); err != nil {
		t.Fatalf("StopAsync error: %v", err)
	}
	if bc.Status() != enums.PluginStopped {
		t.Errorf("expected PluginStopped, got %v", bc.Status())
	}
}

// ---------------------------------------------------------------------------
// 6. TestBaseConnector_Reconnection_Success
// ---------------------------------------------------------------------------

func TestBaseConnector_Reconnection_Success(t *testing.T) {
	bc, bus := newTestConnector()
	ctx := context.Background()

	// Subscribe to provider events.
	_, provCh := bus.Providers.Subscribe(16)

	// Set up reconnection: first call fails, second succeeds.
	var callCount int32
	bc.SetReconnectionAction(func(ctx context.Context) error {
		n := atomic.AddInt32(&callCount, 1)
		if n == 1 {
			return errors.New("simulated failure")
		}
		return nil
	})

	bc.HandleConnectionLost(ctx, "test disconnect", errors.New("test error"))

	// Wait for reconnection to complete (with timeout).
	deadline := time.After(30 * time.Second)
	var lastProvider models.Provider
	for {
		select {
		case p := <-provCh:
			lastProvider = p
			if p.Status == enums.SessionConnected {
				goto done
			}
		case <-deadline:
			t.Fatal("timed out waiting for successful reconnection")
		}
	}
done:
	if lastProvider.Status != enums.SessionConnected {
		t.Errorf("expected SessionConnected, got %v", lastProvider.Status)
	}

	finalCount := atomic.LoadInt32(&callCount)
	if finalCount < 2 {
		t.Errorf("expected at least 2 reconnect calls, got %d", finalCount)
	}
}

// ---------------------------------------------------------------------------
// 7. TestBaseConnector_Reconnection_MaxAttempts
// ---------------------------------------------------------------------------

func TestBaseConnector_Reconnection_MaxAttempts(t *testing.T) {
	bc, bus := newTestConnector()
	// Use a small max for faster test.
	bc.maxReconnectAttempts = 2
	ctx := context.Background()

	_, provCh := bus.Providers.Subscribe(16)

	// Always fail.
	bc.SetReconnectionAction(func(ctx context.Context) error {
		return errors.New("always fail")
	})

	bc.HandleConnectionLost(ctx, "test disconnect", errors.New("test"))

	deadline := time.After(30 * time.Second)
	var lastProvider models.Provider
	for {
		select {
		case p := <-provCh:
			lastProvider = p
			if p.Status == enums.SessionDisconnectedFailed {
				goto done
			}
		case <-deadline:
			t.Fatal("timed out waiting for reconnection exhaustion")
		}
	}
done:
	if lastProvider.Status != enums.SessionDisconnectedFailed {
		t.Errorf("expected SessionDisconnectedFailed, got %v", lastProvider.Status)
	}
	if bc.Status() != enums.PluginStoppedFailed {
		t.Errorf("expected PluginStoppedFailed, got %v", bc.Status())
	}
}

// ---------------------------------------------------------------------------
// 8. TestBaseConnector_Reconnection_ConcurrentCalls
// ---------------------------------------------------------------------------

func TestBaseConnector_Reconnection_ConcurrentCalls(t *testing.T) {
	bc, _ := newTestConnector()
	bc.maxReconnectAttempts = 1
	ctx := context.Background()

	var callCount int32
	bc.SetReconnectionAction(func(ctx context.Context) error {
		atomic.AddInt32(&callCount, 1)
		return nil
	})

	// Fire 10 concurrent HandleConnectionLost calls.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			bc.HandleConnectionLost(ctx, fmt.Sprintf("concurrent-%d", n), nil)
		}(i)
	}
	wg.Wait()

	// Give the goroutine that entered the reconnection loop time to finish.
	time.Sleep(5 * time.Second)

	count := atomic.LoadInt32(&callCount)
	// At least one goroutine must have executed the reconnection.
	if count < 1 {
		t.Errorf("expected at least 1 reconnect call, got %d", count)
	}
	// The CAS guard should prevent truly concurrent execution. However, if the
	// first goroutine finishes and resets isReconnecting before all others run,
	// a second one could enter. So we allow a small number but log it.
	if count > 2 {
		t.Errorf("expected at most 2 reconnect executions (one initial + one after reset), got %d", count)
	}
	t.Logf("reconnect function called %d time(s)", count)
}

// ---------------------------------------------------------------------------
// 9. TestBaseConnector_PluginUniqueID
// ---------------------------------------------------------------------------

func TestBaseConnector_PluginUniqueID(t *testing.T) {
	bc, _ := newTestConnector()

	// Compute expected SHA256.
	input := "TestConnector" + "TestAuthor" + "1.0.0" + "A test connector"
	h := sha256.New()
	h.Write([]byte(input))
	expected := fmt.Sprintf("%x", h.Sum(nil))

	got := bc.PluginUniqueID()
	if got != expected {
		t.Errorf("PluginUniqueID mismatch:\n  got  %s\n  want %s", got, expected)
	}

	// Create another connector with same fields: ID should be identical.
	bc2, _ := newTestConnector()
	if bc.PluginUniqueID() != bc2.PluginUniqueID() {
		t.Error("identical connectors should produce the same PluginUniqueID")
	}
}

// ---------------------------------------------------------------------------
// 10. TestBaseConnector_PublishOrderBook
// ---------------------------------------------------------------------------

func TestBaseConnector_PublishOrderBook(t *testing.T) {
	bc, bus := newTestConnector()

	_, ch := bus.OrderBooks.Subscribe(4)

	ob := models.NewOrderBook("BTC/USDT", 2, 10)
	ob.ProviderID = 42
	ob.ProviderName = "TestProvider"

	bc.PublishOrderBook(ob)

	select {
	case received := <-ch:
		if received.Symbol != "BTC/USDT" {
			t.Errorf("expected symbol BTC/USDT, got %s", received.Symbol)
		}
		if received.ProviderID != 42 {
			t.Errorf("expected providerID 42, got %d", received.ProviderID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for OrderBook event")
	}
}
