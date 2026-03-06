package scanner

import (
	"math"
	"testing"
)

func TestCalcPivots(t *testing.T) {
	// Known values: H=110, L=90, C=100
	p := CalcPivots(110, 90, 100)

	// P = (110+90+100)/3 = 100
	if math.Abs(p.P-100) > 0.01 {
		t.Errorf("P expected 100, got %f", p.P)
	}

	// S1 = 2*100 - 110 = 90
	if math.Abs(p.S1-90) > 0.01 {
		t.Errorf("S1 expected 90, got %f", p.S1)
	}

	// R1 = 2*100 - 90 = 110
	if math.Abs(p.R1-110) > 0.01 {
		t.Errorf("R1 expected 110, got %f", p.R1)
	}

	// S2 = 100 - (110-90) = 80
	if math.Abs(p.S2-80) > 0.01 {
		t.Errorf("S2 expected 80, got %f", p.S2)
	}

	// R2 = 100 + (110-90) = 120
	if math.Abs(p.R2-120) > 0.01 {
		t.Errorf("R2 expected 120, got %f", p.R2)
	}

	// S3 = 90 - 2*(110-100) = 70
	if math.Abs(p.S3-70) > 0.01 {
		t.Errorf("S3 expected 70, got %f", p.S3)
	}

	// R3 = 110 + 2*(100-90) = 130
	if math.Abs(p.R3-130) > 0.01 {
		t.Errorf("R3 expected 130, got %f", p.R3)
	}
}

func TestClassifyPivotWidth(t *testing.T) {
	tests := []struct {
		s1, r1, price float64
		expected      string
	}{
		{99.5, 100.5, 100, "Narrow"},  // 1% range — well under 2%
		{97, 103, 100, "Wide"},    // 6% range
		{98, 102, 100, "Normal"},  // 4% range
		{95, 105, 100, "Wide"},    // 10% range
		{99.2, 100.8, 100, "Narrow"}, // 1.6% range
	}

	for _, tt := range tests {
		result := ClassifyPivotWidth(tt.s1, tt.r1, tt.price)
		if result != tt.expected {
			t.Errorf("ClassifyPivotWidth(%f, %f, %f) = %s, want %s",
				tt.s1, tt.r1, tt.price, result, tt.expected)
		}
	}
}

func TestFindNearestPivot(t *testing.T) {
	pivots := CalcPivots(110, 90, 100) // P=100, S1=90, R1=110
	result := FindNearestPivot(pivots, 99)

	// Nearest should be P at 100
	if result.Level != "Pivot" {
		t.Errorf("expected Pivot, got %s", result.Level)
	}
}
