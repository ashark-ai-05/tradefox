package scanner

import "testing"

func TestCalcOIChange_KnownData(t *testing.T) {
	history := []OIPoint{
		{OI: 1000, Timestamp: 0},
		{OI: 1020, Timestamp: 300000},
		{OI: 1050, Timestamp: 600000},
		{OI: 1080, Timestamp: 900000},
		{OI: 1100, Timestamp: 1200000},
		{OI: 1130, Timestamp: 1500000},
		{OI: 1150, Timestamp: 1800000},
	}
	currentOI := 1200.0

	result := CalcOIChange(history, currentOI)

	if result.RawOI != 1200 {
		t.Errorf("expected RawOI=1200, got %f", result.RawOI)
	}

	// change24H should use first point: (1200-1000)/1000 * 100 = 20%
	if result.Change24H < 19.9 || result.Change24H > 20.1 {
		t.Errorf("expected Change24H ~20%%, got %f%%", result.Change24H)
	}
}

func TestCalcOIChange_EmptyHistory(t *testing.T) {
	result := CalcOIChange(nil, 100)
	if result.State != "Stable" {
		t.Errorf("expected Stable state for empty history, got %s", result.State)
	}
	if result.RawOI != 100 {
		t.Errorf("expected RawOI=100, got %f", result.RawOI)
	}
}

func TestCalcOIChange_Building(t *testing.T) {
	// Create history where 4h change > 5%
	history := []OIPoint{
		{OI: 100, Timestamp: 0},
		{OI: 100, Timestamp: 300000},
		{OI: 100, Timestamp: 600000},
		{OI: 100, Timestamp: 900000},
		{OI: 100, Timestamp: 1200000},
		{OI: 102, Timestamp: 1500000},
		{OI: 104, Timestamp: 1800000},
	}
	// 4h change uses index len-5: (120-100)/100 = 20%
	result := CalcOIChange(history, 120)
	if result.State != "Building" {
		t.Errorf("expected Building state, got %s", result.State)
	}
}

func TestCalcOIChange_Declining(t *testing.T) {
	history := []OIPoint{
		{OI: 200, Timestamp: 0},
		{OI: 195, Timestamp: 300000},
		{OI: 190, Timestamp: 600000},
		{OI: 185, Timestamp: 900000},
		{OI: 180, Timestamp: 1200000},
		{OI: 175, Timestamp: 1500000},
		{OI: 170, Timestamp: 1800000},
	}
	// 4h change uses index len-5=2: (150-190)/190 = ~-21%
	result := CalcOIChange(history, 150)
	if result.State != "Declining" {
		t.Errorf("expected Declining state, got %s", result.State)
	}
}
