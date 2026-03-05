package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/ashark-ai-05/tradefox/internal/core/enums"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func float64Ptr(v float64) *float64 { return &v }
func boolPtr(v bool) *bool          { return &v }

// ---------------------------------------------------------------------------
// BookItem tests
// ---------------------------------------------------------------------------

func TestBookItem_CopyFrom(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	src := &BookItem{
		Symbol:             "AAPL",
		ProviderID:         1,
		EntryID:            "e1",
		LayerName:          "L1",
		LocalTimestamp:      now,
		ServerTimestamp:     now.Add(-time.Millisecond),
		Price:              float64Ptr(150.25),
		Size:               float64Ptr(100),
		IsBid:              true,
		CumulativeSize:     float64Ptr(500),
		ActiveSize:         float64Ptr(80),
		PriceDecimalPlaces: 2,
		SizeDecimalPlaces:  0,
	}

	dst := &BookItem{}
	dst.CopyFrom(src)

	// Verify all fields match.
	if dst.Symbol != src.Symbol {
		t.Errorf("Symbol: got %q, want %q", dst.Symbol, src.Symbol)
	}
	if dst.ProviderID != src.ProviderID {
		t.Errorf("ProviderID: got %d, want %d", dst.ProviderID, src.ProviderID)
	}
	if dst.EntryID != src.EntryID {
		t.Errorf("EntryID: got %q, want %q", dst.EntryID, src.EntryID)
	}
	if dst.LayerName != src.LayerName {
		t.Errorf("LayerName: got %q, want %q", dst.LayerName, src.LayerName)
	}
	if !dst.LocalTimestamp.Equal(src.LocalTimestamp) {
		t.Errorf("LocalTimestamp: got %v, want %v", dst.LocalTimestamp, src.LocalTimestamp)
	}
	if !dst.ServerTimestamp.Equal(src.ServerTimestamp) {
		t.Errorf("ServerTimestamp: got %v, want %v", dst.ServerTimestamp, src.ServerTimestamp)
	}
	if *dst.Price != *src.Price {
		t.Errorf("Price: got %v, want %v", *dst.Price, *src.Price)
	}
	if *dst.Size != *src.Size {
		t.Errorf("Size: got %v, want %v", *dst.Size, *src.Size)
	}
	if dst.IsBid != src.IsBid {
		t.Errorf("IsBid: got %v, want %v", dst.IsBid, src.IsBid)
	}
	if *dst.CumulativeSize != *src.CumulativeSize {
		t.Errorf("CumulativeSize: got %v, want %v", *dst.CumulativeSize, *src.CumulativeSize)
	}
	if *dst.ActiveSize != *src.ActiveSize {
		t.Errorf("ActiveSize: got %v, want %v", *dst.ActiveSize, *src.ActiveSize)
	}
	if dst.PriceDecimalPlaces != src.PriceDecimalPlaces {
		t.Errorf("PriceDecimalPlaces: got %d, want %d", dst.PriceDecimalPlaces, src.PriceDecimalPlaces)
	}
	if dst.SizeDecimalPlaces != src.SizeDecimalPlaces {
		t.Errorf("SizeDecimalPlaces: got %d, want %d", dst.SizeDecimalPlaces, src.SizeDecimalPlaces)
	}

	// Verify deep copy (mutating src pointer values must not affect dst).
	*src.Price = 999.0
	if *dst.Price == 999.0 {
		t.Error("CopyFrom did not deep-copy Price pointer")
	}
}

func TestBookItem_CopyEssentials(t *testing.T) {
	src := &BookItem{
		Symbol:         "MSFT",
		ProviderID:     2,
		EntryID:        "e2",
		Price:          float64Ptr(300.50),
		Size:           float64Ptr(200),
		IsBid:          false,
		ActiveSize:     float64Ptr(150),
		CumulativeSize: float64Ptr(1000),
	}

	dst := &BookItem{
		Symbol:     "OLD",
		ProviderID: 99,
		EntryID:    "old",
	}
	dst.CopyEssentials(src)

	// Essential fields must be copied.
	if *dst.Price != *src.Price {
		t.Errorf("Price: got %v, want %v", *dst.Price, *src.Price)
	}
	if *dst.Size != *src.Size {
		t.Errorf("Size: got %v, want %v", *dst.Size, *src.Size)
	}
	if dst.IsBid != src.IsBid {
		t.Errorf("IsBid: got %v, want %v", dst.IsBid, src.IsBid)
	}
	if *dst.ActiveSize != *src.ActiveSize {
		t.Errorf("ActiveSize: got %v, want %v", *dst.ActiveSize, *src.ActiveSize)
	}
	if *dst.CumulativeSize != *src.CumulativeSize {
		t.Errorf("CumulativeSize: got %v, want %v", *dst.CumulativeSize, *src.CumulativeSize)
	}

	// Non-essential fields must NOT be overwritten.
	if dst.Symbol != "OLD" {
		t.Errorf("Symbol should not be copied, got %q", dst.Symbol)
	}
	if dst.ProviderID != 99 {
		t.Errorf("ProviderID should not be copied, got %d", dst.ProviderID)
	}
	if dst.EntryID != "old" {
		t.Errorf("EntryID should not be copied, got %q", dst.EntryID)
	}
}

func TestBookItem_Reset(t *testing.T) {
	b := &BookItem{
		Symbol:     "GOOG",
		ProviderID: 5,
		Price:      float64Ptr(2800),
		Size:       float64Ptr(50),
		IsBid:      true,
	}
	b.Reset()

	if b.Symbol != "" {
		t.Errorf("Symbol should be empty after Reset, got %q", b.Symbol)
	}
	if b.ProviderID != 0 {
		t.Errorf("ProviderID should be 0 after Reset, got %d", b.ProviderID)
	}
	if b.Price != nil {
		t.Error("Price should be nil after Reset")
	}
	if b.Size != nil {
		t.Error("Size should be nil after Reset")
	}
	if b.IsBid {
		t.Error("IsBid should be false after Reset")
	}
}

func TestBookItem_Equals(t *testing.T) {
	a := &BookItem{IsBid: true, EntryID: "e1", Price: float64Ptr(100), Size: float64Ptr(10)}
	b := &BookItem{IsBid: true, EntryID: "e1", Price: float64Ptr(100), Size: float64Ptr(10)}

	if !a.Equals(b) {
		t.Error("identical BookItems should be equal")
	}

	// Differ by IsBid.
	c := &BookItem{IsBid: false, EntryID: "e1", Price: float64Ptr(100), Size: float64Ptr(10)}
	if a.Equals(c) {
		t.Error("should not be equal when IsBid differs")
	}

	// Differ by Price.
	d := &BookItem{IsBid: true, EntryID: "e1", Price: float64Ptr(101), Size: float64Ptr(10)}
	if a.Equals(d) {
		t.Error("should not be equal when Price differs")
	}

	// Nil Price vs non-nil.
	e := &BookItem{IsBid: true, EntryID: "e1", Price: nil, Size: float64Ptr(10)}
	if a.Equals(e) {
		t.Error("should not be equal when one Price is nil")
	}

	// Nil other.
	if a.Equals(nil) {
		t.Error("should not be equal to nil")
	}
}

func TestBookItem_FormattedSize(t *testing.T) {
	tests := []struct {
		size *float64
		want string
	}{
		{float64Ptr(1500), "1.5K"},
		{float64Ptr(2500000), "2.5M"},
		{float64Ptr(1000), "1K"},
		{float64Ptr(1000000), "1M"},
		{float64Ptr(500), "500"},
		{float64Ptr(0), "0"},
		{nil, ""},
	}
	for _, tt := range tests {
		b := &BookItem{Size: tt.size}
		got := b.FormattedSize()
		if got != tt.want {
			sizeStr := "<nil>"
			if tt.size != nil {
				sizeStr = decimal.NewFromFloat(*tt.size).String()
			}
			t.Errorf("FormattedSize(%s) = %q, want %q", sizeStr, got, tt.want)
		}
	}
}

func TestBookItem_FormattedPrice(t *testing.T) {
	b := &BookItem{Price: float64Ptr(123.456789), PriceDecimalPlaces: 4}
	got := b.FormattedPrice()
	want := "123.4568"
	if got != want {
		t.Errorf("FormattedPrice() = %q, want %q", got, want)
	}

	b2 := &BookItem{Price: nil}
	if b2.FormattedPrice() != "" {
		t.Error("FormattedPrice() should return empty string for nil Price")
	}
}

// ---------------------------------------------------------------------------
// Trade tests
// ---------------------------------------------------------------------------

func TestTrade_CopyTo(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	src := &Trade{
		ProviderID:     1,
		ProviderName:   "Binance",
		Symbol:         "BTC/USD",
		Price:          decimal.NewFromFloat(42000.50),
		Size:           decimal.NewFromFloat(1.5),
		Timestamp:      now,
		IsBuy:          boolPtr(true),
		Flags:          "aggressive",
		MarketMidPrice: 42000.25,
	}

	dst := &Trade{}
	src.CopyTo(dst)

	if dst.ProviderID != src.ProviderID {
		t.Errorf("ProviderID: got %d, want %d", dst.ProviderID, src.ProviderID)
	}
	if dst.ProviderName != src.ProviderName {
		t.Errorf("ProviderName: got %q, want %q", dst.ProviderName, src.ProviderName)
	}
	if dst.Symbol != src.Symbol {
		t.Errorf("Symbol: got %q, want %q", dst.Symbol, src.Symbol)
	}
	if !dst.Price.Equal(src.Price) {
		t.Errorf("Price: got %s, want %s", dst.Price, src.Price)
	}
	if !dst.Size.Equal(src.Size) {
		t.Errorf("Size: got %s, want %s", dst.Size, src.Size)
	}
	if !dst.Timestamp.Equal(src.Timestamp) {
		t.Errorf("Timestamp: got %v, want %v", dst.Timestamp, src.Timestamp)
	}
	if dst.IsBuy == nil || *dst.IsBuy != true {
		t.Errorf("IsBuy: got %v, want true", dst.IsBuy)
	}
	if dst.Flags != src.Flags {
		t.Errorf("Flags: got %q, want %q", dst.Flags, src.Flags)
	}
	if dst.MarketMidPrice != src.MarketMidPrice {
		t.Errorf("MarketMidPrice: got %v, want %v", dst.MarketMidPrice, src.MarketMidPrice)
	}

	// Verify deep copy of IsBuy pointer.
	*src.IsBuy = false
	if *dst.IsBuy == false {
		t.Error("CopyTo did not deep-copy IsBuy pointer")
	}
}

// ---------------------------------------------------------------------------
// Provider tests
// ---------------------------------------------------------------------------

func TestProvider_Tooltip(t *testing.T) {
	tests := []struct {
		status enums.SessionStatus
		want   string
	}{
		{enums.SessionConnecting, "Connecting..."},
		{enums.SessionConnected, "Connected"},
		{enums.SessionConnectedWithWarnings, "Connected with limitations"},
		{enums.SessionDisconnectedFailed, "Failure Disconnection"},
		{enums.SessionDisconnected, "Disconnected"},
		{enums.SessionStatus(99), "Unknown"},
	}
	for _, tt := range tests {
		p := &Provider{Status: tt.status}
		got := p.Tooltip()
		if got != tt.want {
			t.Errorf("Tooltip(%v) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Execution tests
// ---------------------------------------------------------------------------

func TestExecution_Latency(t *testing.T) {
	server := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	local := server.Add(5 * time.Millisecond)

	e := &Execution{
		ServerTimestamp: server,
		LocalTimestamp:  local,
	}

	got := e.Latency()
	want := 5 * time.Millisecond
	if got != want {
		t.Errorf("Latency() = %v, want %v", got, want)
	}
}

func TestExecution_Latency_Negative(t *testing.T) {
	// When local is before server (clock skew), latency is negative.
	server := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	local := server.Add(-2 * time.Millisecond)

	e := &Execution{
		ServerTimestamp: server,
		LocalTimestamp:  local,
	}

	got := e.Latency()
	want := -2 * time.Millisecond
	if got != want {
		t.Errorf("Latency() = %v, want %v", got, want)
	}
}

// ---------------------------------------------------------------------------
// JSON round-trip tests
// ---------------------------------------------------------------------------

func TestTrade_JSONRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond).UTC()
	original := &Trade{
		ProviderID:     3,
		ProviderName:   "Kraken",
		Symbol:         "ETH/USD",
		Price:          decimal.NewFromFloat(3150.75),
		Size:           decimal.NewFromFloat(0.25),
		Timestamp:      now,
		IsBuy:          boolPtr(false),
		Flags:          "passive",
		MarketMidPrice: 3150.50,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var restored Trade
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if restored.ProviderID != original.ProviderID {
		t.Errorf("ProviderID: got %d, want %d", restored.ProviderID, original.ProviderID)
	}
	if restored.ProviderName != original.ProviderName {
		t.Errorf("ProviderName: got %q, want %q", restored.ProviderName, original.ProviderName)
	}
	if restored.Symbol != original.Symbol {
		t.Errorf("Symbol: got %q, want %q", restored.Symbol, original.Symbol)
	}
	if !restored.Price.Equal(original.Price) {
		t.Errorf("Price: got %s, want %s", restored.Price, original.Price)
	}
	if !restored.Size.Equal(original.Size) {
		t.Errorf("Size: got %s, want %s", restored.Size, original.Size)
	}
	if !restored.Timestamp.Equal(original.Timestamp) {
		t.Errorf("Timestamp: got %v, want %v", restored.Timestamp, original.Timestamp)
	}
	if restored.IsBuy == nil || *restored.IsBuy != *original.IsBuy {
		t.Errorf("IsBuy: got %v, want %v", restored.IsBuy, original.IsBuy)
	}
	if restored.Flags != original.Flags {
		t.Errorf("Flags: got %q, want %q", restored.Flags, original.Flags)
	}
	if restored.MarketMidPrice != original.MarketMidPrice {
		t.Errorf("MarketMidPrice: got %v, want %v", restored.MarketMidPrice, original.MarketMidPrice)
	}
}

func TestProvider_JSONRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond).UTC()
	original := &Provider{
		ProviderID:   7,
		ProviderCode: 42,
		ProviderName: "TestProvider",
		Status:       enums.SessionConnected,
		LastUpdated:  now,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var restored Provider
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if restored.ProviderID != original.ProviderID {
		t.Errorf("ProviderID: got %d, want %d", restored.ProviderID, original.ProviderID)
	}
	if restored.ProviderCode != original.ProviderCode {
		t.Errorf("ProviderCode: got %d, want %d", restored.ProviderCode, original.ProviderCode)
	}
	if restored.ProviderName != original.ProviderName {
		t.Errorf("ProviderName: got %q, want %q", restored.ProviderName, original.ProviderName)
	}
	if restored.Status != original.Status {
		t.Errorf("Status: got %v, want %v", restored.Status, original.Status)
	}
	if !restored.LastUpdated.Equal(original.LastUpdated) {
		t.Errorf("LastUpdated: got %v, want %v", restored.LastUpdated, original.LastUpdated)
	}
}
