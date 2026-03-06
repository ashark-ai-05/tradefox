package scanner

import "testing"

func TestClassifyFunding(t *testing.T) {
	tests := []struct {
		name     string
		rate     float64
		expected string
	}{
		{"zero", 0, "Normal"},
		{"positive normal", 0.00005, "Normal"},
		{"negative normal", -0.00005, "Normal"},
		{"positive elevated lower bound", 0.00011, "Elevated"},
		{"positive elevated", 0.0002, "Elevated"},
		{"negative elevated", -0.0002, "NegElevated"},
		{"positive high lower bound", 0.00031, "High"},
		{"positive high", 0.0004, "High"},
		{"negative high", -0.0004, "NegHigh"},
		{"positive extreme", 0.001, "Extreme"},
		{"negative extreme", -0.001, "NegExtreme"},
		// Boundary tests
		{"exactly 0.01% positive", 0.0001, "Normal"},
		{"just above 0.01% positive", 0.000101, "Elevated"},
		{"exactly 0.03% positive", 0.0003, "Elevated"},
		{"just above 0.03% positive", 0.000301, "High"},
		{"exactly 0.05% positive", 0.0005, "High"},
		{"just above 0.05% positive", 0.000501, "Extreme"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyFunding(tt.rate)
			if result != tt.expected {
				t.Errorf("ClassifyFunding(%f) = %s, want %s", tt.rate, result, tt.expected)
			}
		})
	}
}
