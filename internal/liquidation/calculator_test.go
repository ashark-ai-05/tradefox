package liquidation

import (
	"math"
	"testing"
)

func TestCalcLiquidationPrice_Longs(t *testing.T) {
	tests := []struct {
		entry    float64
		leverage float64
		want     float64
	}{
		{100000, 3, 100000 * (1.0 - 1.0/3.0)},
		{100000, 5, 100000 * (1.0 - 1.0/5.0)},
		{100000, 10, 100000 * (1.0 - 1.0/10.0)},
		{100000, 20, 100000 * (1.0 - 1.0/20.0)},
		{100000, 50, 100000 * (1.0 - 1.0/50.0)},
		{100000, 100, 100000 * (1.0 - 1.0/100.0)},
	}

	for _, tt := range tests {
		got := CalcLiquidationPrice(tt.entry, tt.leverage, "long")
		if math.Abs(got-tt.want) > 0.01 {
			t.Errorf("CalcLiquidationPrice(%v, %v, long) = %v, want %v", tt.entry, tt.leverage, got, tt.want)
		}
	}
}

func TestCalcLiquidationPrice_Shorts(t *testing.T) {
	tests := []struct {
		entry    float64
		leverage float64
		want     float64
	}{
		{100000, 3, 100000 * (1.0 + 1.0/3.0)},
		{100000, 5, 100000 * (1.0 + 1.0/5.0)},
		{100000, 10, 100000 * (1.0 + 1.0/10.0)},
		{100000, 25, 100000 * (1.0 + 1.0/25.0)},
		{100000, 100, 100000 * (1.0 + 1.0/100.0)},
	}

	for _, tt := range tests {
		got := CalcLiquidationPrice(tt.entry, tt.leverage, "short")
		if math.Abs(got-tt.want) > 0.01 {
			t.Errorf("CalcLiquidationPrice(%v, %v, short) = %v, want %v", tt.entry, tt.leverage, got, tt.want)
		}
	}
}

func TestCalcLiquidationPrice_ZeroLeverage(t *testing.T) {
	got := CalcLiquidationPrice(100000, 0, "long")
	if got != 0 {
		t.Errorf("expected 0 for zero leverage, got %v", got)
	}
}

func TestCalcLiquidationPrice_InvalidSide(t *testing.T) {
	got := CalcLiquidationPrice(100000, 10, "invalid")
	if got != 0 {
		t.Errorf("expected 0 for invalid side, got %v", got)
	}
}

func TestFanOutLiquidations(t *testing.T) {
	pos := PositionEstimate{
		EntryPrice: 100000,
		Volume:     1000,
		Timestamp:  1000,
		Side:       "long",
	}

	levels := FanOutLiquidations(pos)

	// Should produce one level per leverage tier
	if len(levels) != len(defaultLeverageTiers) {
		t.Fatalf("expected %d levels, got %d", len(defaultLeverageTiers), len(levels))
	}

	// Verify weights sum to total volume
	var totalVol float64
	for _, l := range levels {
		totalVol += l.Volume
		if l.Side != "long" {
			t.Errorf("expected side 'long', got %q", l.Side)
		}
		if l.EntryPrice != 100000 {
			t.Errorf("expected entry 100000, got %v", l.EntryPrice)
		}
		if l.Price >= 100000 {
			t.Errorf("long liquidation should be below entry, got %v", l.Price)
		}
	}

	if math.Abs(totalVol-1000) > 0.01 {
		t.Errorf("total volume %v should equal position volume 1000", totalVol)
	}
}

func TestFanOutLiquidations_Short(t *testing.T) {
	pos := PositionEstimate{
		EntryPrice: 50000,
		Volume:     500,
		Timestamp:  2000,
		Side:       "short",
	}

	levels := FanOutLiquidations(pos)

	for _, l := range levels {
		if l.Price <= 50000 {
			t.Errorf("short liquidation should be above entry, got %v", l.Price)
		}
	}
}

func TestAggregateLiquidations(t *testing.T) {
	levels := []LiquidationLevel{
		{Price: 95000, Volume: 100, Side: "long", Leverage: 10, EntryPrice: 100000},
		{Price: 95100, Volume: 200, Side: "long", Leverage: 10, EntryPrice: 100000},
		{Price: 105000, Volume: 150, Side: "short", Leverage: 10, EntryPrice: 100000},
	}

	bands := AggregateLiquidations(levels, 100000, 100, 10)

	if bands == nil {
		t.Fatal("expected non-nil bands")
	}
	if len(bands) != 100 {
		t.Fatalf("expected 100 bins, got %d", len(bands))
	}

	// Verify range covers ±10% of 100000
	if math.Abs(bands[0].PriceMin-90000) > 1 {
		t.Errorf("expected first bin to start near 90000, got %v", bands[0].PriceMin)
	}
	if math.Abs(bands[99].PriceMax-110000) > 1 {
		t.Errorf("expected last bin to end near 110000, got %v", bands[99].PriceMax)
	}

	// At least one band should have long volume
	var hasLong, hasShort bool
	for _, b := range bands {
		if b.LongLiqVolume > 0 {
			hasLong = true
		}
		if b.ShortLiqVolume > 0 {
			hasShort = true
		}
	}
	if !hasLong {
		t.Error("expected some bands with long liquidation volume")
	}
	if !hasShort {
		t.Error("expected some bands with short liquidation volume")
	}
}

func TestAggregateLiquidations_InvalidInputs(t *testing.T) {
	if bands := AggregateLiquidations(nil, 100000, 0, 10); bands != nil {
		t.Error("expected nil for zero bins")
	}
	if bands := AggregateLiquidations(nil, 0, 100, 10); bands != nil {
		t.Error("expected nil for zero price")
	}
	if bands := AggregateLiquidations(nil, 100000, 100, 0); bands != nil {
		t.Error("expected nil for zero range")
	}
}
