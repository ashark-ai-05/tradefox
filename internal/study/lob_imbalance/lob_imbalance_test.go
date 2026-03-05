package lobimbalance

import (
	"context"
	"log/slog"
	"math"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/ashark-ai-05/tradefox/internal/config"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func ptrFloat64(v float64) *float64 { return &v }
func ptrBool(v bool) *bool          { return &v }

func testLogger() *slog.Logger {
	return slog.Default()
}

func newTestBus() *eventbus.Bus {
	return eventbus.NewBus(testLogger())
}

func newTestStudy() (*LOBImbalanceStudy, *eventbus.Bus) {
	bus := newTestBus()
	settings := config.NewManager("")
	s := New(bus, settings, testLogger())
	return s, bus
}

// makeDelta creates a DeltaBookItem for the given side, price, and size.
func makeDelta(isBid bool, price, size float64) models.DeltaBookItem {
	return models.DeltaBookItem{
		IsBid:          ptrBool(isBid),
		Price:          ptrFloat64(price),
		Size:           ptrFloat64(size),
		LocalTimestamp:  time.Now(),
		ServerTimestamp: time.Now(),
	}
}

// buildOrderBook creates an order book and populates it with the given
// bid and ask levels. Each level is a [price, size] pair.
func buildOrderBook(bids, asks [][2]float64) *models.OrderBook {
	ob := models.NewOrderBook("BTCUSD", 2, 0)
	for _, b := range bids {
		ob.AddOrUpdateLevel(makeDelta(true, b[0], b[1]))
	}
	for _, a := range asks {
		ob.AddOrUpdateLevel(makeDelta(false, a[0], a[1]))
	}
	return ob
}

// publishAndCollect publishes the order book on the bus and waits for a
// study output on the OnCalculated channel. Returns the resulting model
// or fails the test on timeout.
func publishAndCollect(t *testing.T, s *LOBImbalanceStudy, bus *eventbus.Bus, ob *models.OrderBook) models.BaseStudyModel {
	t.Helper()

	bus.OrderBooks.Publish(ob)

	select {
	case m := <-s.OnCalculated():
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for study output")
		return models.BaseStudyModel{} // unreachable
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestAllVolumeOnBidSide verifies that when all volume is on the bid side
// and the ask side is empty, the imbalance is +1.0.
func TestAllVolumeOnBidSide(t *testing.T) {
	s, bus := newTestStudy()
	s.SetBookDepth(5)
	s.SetStaleDuration(10 * time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer s.StopAsync(ctx)

	// Order book with only bid levels.
	ob := buildOrderBook(
		[][2]float64{{100.0, 10.0}, {99.0, 20.0}, {98.0, 30.0}},
		nil,
	)

	// With zero ask volume the denominator would be totalBid + 0 = totalBid,
	// and the imbalance would be totalBid / totalBid = 1.0. However the
	// handleOrderBook implementation already checks total == 0 and returns
	// early. Here total != 0 because totalBid > 0, so we do get a result.
	bus.OrderBooks.Publish(ob)

	select {
	case m := <-s.OnCalculated():
		if !m.Value.Equal(decimal.NewFromInt(1)) {
			t.Fatalf("expected imbalance +1.0, got %s", m.Value)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for study output")
	}
}

// TestAllVolumeOnAskSide verifies that when all volume is on the ask side
// and the bid side is empty, the imbalance is -1.0.
func TestAllVolumeOnAskSide(t *testing.T) {
	s, bus := newTestStudy()
	s.SetBookDepth(5)
	s.SetStaleDuration(10 * time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer s.StopAsync(ctx)

	// Order book with only ask levels.
	ob := buildOrderBook(
		nil,
		[][2]float64{{101.0, 15.0}, {102.0, 25.0}},
	)

	bus.OrderBooks.Publish(ob)

	select {
	case m := <-s.OnCalculated():
		if !m.Value.Equal(decimal.NewFromInt(-1)) {
			t.Fatalf("expected imbalance -1.0, got %s", m.Value)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for study output")
	}
}

// TestEqualVolume verifies that when bid and ask volume are equal the
// imbalance is 0.0.
func TestEqualVolume(t *testing.T) {
	s, bus := newTestStudy()
	s.SetBookDepth(5)
	s.SetStaleDuration(10 * time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer s.StopAsync(ctx)

	// Equal bid and ask volume.
	ob := buildOrderBook(
		[][2]float64{{100.0, 50.0}},
		[][2]float64{{101.0, 50.0}},
	)

	m := publishAndCollect(t, s, bus, ob)
	if !m.Value.Equal(decimal.Zero) {
		t.Fatalf("expected imbalance 0.0, got %s", m.Value)
	}
}

// TestAsymmetricLevels verifies the imbalance calculation with 5 bid levels
// and 3 ask levels of varying sizes.
func TestAsymmetricLevels(t *testing.T) {
	s, bus := newTestStudy()
	s.SetBookDepth(5)
	s.SetStaleDuration(10 * time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer s.StopAsync(ctx)

	// 5 bid levels: sizes 10 + 20 + 30 + 40 + 50 = 150
	// 3 ask levels: sizes 5 + 15 + 25 = 45
	// Imbalance = (150 - 45) / (150 + 45) = 105 / 195 = 0.538461538...
	ob := buildOrderBook(
		[][2]float64{
			{100.0, 10.0},
			{99.0, 20.0},
			{98.0, 30.0},
			{97.0, 40.0},
			{96.0, 50.0},
		},
		[][2]float64{
			{101.0, 5.0},
			{102.0, 15.0},
			{103.0, 25.0},
		},
	)

	m := publishAndCollect(t, s, bus, ob)

	// Expected: 105 / 195
	expected := 105.0 / 195.0
	got, _ := m.Value.Float64()
	if math.Abs(got-expected) > 1e-9 {
		t.Fatalf("expected imbalance ~%f, got %f", expected, got)
	}
}

// TestEmptyOrderBook verifies that an empty order book does not cause a
// crash and does not produce an output (total volume is zero, so the
// calculation is skipped).
func TestEmptyOrderBook(t *testing.T) {
	s, bus := newTestStudy()
	s.SetBookDepth(5)
	s.SetStaleDuration(10 * time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer s.StopAsync(ctx)

	// Empty order book - no bids or asks.
	ob := models.NewOrderBook("BTCUSD", 2, 0)

	bus.OrderBooks.Publish(ob)

	// The study should skip the empty book. We verify no output arrives
	// by waiting briefly, then confirming the channel is empty.
	select {
	case m := <-s.OnCalculated():
		t.Fatalf("expected no output for empty book, got %s", m.Value)
	case <-time.After(200 * time.Millisecond):
		// Expected: no output.
	}

	// Also test a nil order book pointer - should not crash.
	bus.OrderBooks.Publish(nil)

	select {
	case m := <-s.OnCalculated():
		t.Fatalf("expected no output for nil book, got %s", m.Value)
	case <-time.After(200 * time.Millisecond):
		// Expected: no output.
	}
}

// TestConstructorAndMetadata verifies that the study's metadata fields,
// tile properties, and default configuration are correctly set by the
// constructor.
func TestConstructorAndMetadata(t *testing.T) {
	s, _ := newTestStudy()

	if s.Name() != "LOBImbalance" {
		t.Errorf("expected Name 'LOBImbalance', got %q", s.Name())
	}
	if s.Version() != "1.0.0" {
		t.Errorf("expected Version '1.0.0', got %q", s.Version())
	}
	if s.Description() != "Limit Order Book Imbalance" {
		t.Errorf("expected Description 'Limit Order Book Imbalance', got %q", s.Description())
	}
	if s.Author() != "VisualHFT" {
		t.Errorf("expected Author 'VisualHFT', got %q", s.Author())
	}
	if s.TileTitle() != "LOB Imbalance" {
		t.Errorf("expected TileTitle 'LOB Imbalance', got %q", s.TileTitle())
	}
	if s.TileToolTip() == "" {
		t.Error("expected non-empty TileToolTip")
	}
	if s.BookDepth() != DefaultBookDepth {
		t.Errorf("expected default BookDepth %d, got %d", DefaultBookDepth, s.BookDepth())
	}
	if s.PluginUniqueID() == "" {
		t.Error("expected non-empty PluginUniqueID")
	}

	// Verify SetBookDepth works.
	s.SetBookDepth(10)
	if s.BookDepth() != 10 {
		t.Errorf("expected BookDepth 10 after SetBookDepth, got %d", s.BookDepth())
	}

	// Verify SetBookDepth ignores non-positive values.
	s.SetBookDepth(0)
	if s.BookDepth() != 10 {
		t.Errorf("expected BookDepth still 10 after SetBookDepth(0), got %d", s.BookDepth())
	}
	s.SetBookDepth(-1)
	if s.BookDepth() != 10 {
		t.Errorf("expected BookDepth still 10 after SetBookDepth(-1), got %d", s.BookDepth())
	}
}

// TestBookDepthLimitsLevels verifies that only the top N levels are used
// even when the order book has more levels.
func TestBookDepthLimitsLevels(t *testing.T) {
	s, bus := newTestStudy()
	s.SetBookDepth(2) // Only consider top 2 levels.
	s.SetStaleDuration(10 * time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer s.StopAsync(ctx)

	// 4 bid levels, 4 ask levels. Only top 2 of each should be used.
	// Top 2 bids (highest prices): 100.0@10, 99.0@20 => total = 30
	// Top 2 asks (lowest prices): 101.0@5, 102.0@15 => total = 20
	// Imbalance = (30 - 20) / (30 + 20) = 10 / 50 = 0.2
	ob := buildOrderBook(
		[][2]float64{
			{100.0, 10.0},
			{99.0, 20.0},
			{98.0, 1000.0}, // Should be ignored (beyond depth 2).
			{97.0, 1000.0}, // Should be ignored.
		},
		[][2]float64{
			{101.0, 5.0},
			{102.0, 15.0},
			{103.0, 1000.0}, // Should be ignored.
			{104.0, 1000.0}, // Should be ignored.
		},
	)

	m := publishAndCollect(t, s, bus, ob)

	expected := 10.0 / 50.0
	got, _ := m.Value.Float64()
	if math.Abs(got-expected) > 1e-9 {
		t.Fatalf("expected imbalance %f with depth 2, got %f", expected, got)
	}
}

// TestStudiesTopicPublish verifies that computed imbalance values are
// published to the Studies topic on the event bus.
func TestStudiesTopicPublish(t *testing.T) {
	s, bus := newTestStudy()
	s.SetBookDepth(5)
	s.SetStaleDuration(10 * time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe to the Studies topic before starting.
	_, studiesCh := bus.Studies.Subscribe(64)

	if err := s.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer s.StopAsync(ctx)

	ob := buildOrderBook(
		[][2]float64{{100.0, 30.0}},
		[][2]float64{{101.0, 10.0}},
	)

	bus.OrderBooks.Publish(ob)

	// Should receive on the Studies topic.
	select {
	case m := <-studiesCh:
		// Imbalance = (30 - 10) / (30 + 10) = 20/40 = 0.5
		expected := decimal.NewFromFloat(0.5)
		if !m.Value.Equal(expected) {
			t.Fatalf("expected Studies topic value %s, got %s", expected, m.Value)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Studies topic publish")
	}
}

// TestMarketMidPrice verifies that the mid price from the order book is
// correctly forwarded in the study output.
func TestMarketMidPrice(t *testing.T) {
	s, bus := newTestStudy()
	s.SetBookDepth(5)
	s.SetStaleDuration(10 * time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	defer s.StopAsync(ctx)

	ob := buildOrderBook(
		[][2]float64{{100.0, 10.0}},
		[][2]float64{{102.0, 10.0}},
	)

	m := publishAndCollect(t, s, bus, ob)

	// MidPrice = (100 + 102) / 2 = 101
	expectedMid := 101.0
	if m.MarketMidPrice != expectedMid {
		t.Fatalf("expected MarketMidPrice %f, got %f", expectedMid, m.MarketMidPrice)
	}
}
