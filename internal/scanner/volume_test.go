package scanner

import "testing"

func TestDetectVolumeAnomaly_Normal(t *testing.T) {
	// 20 candles with volume 100, then current with volume 120
	candles := make([]Candle, 21)
	for i := 0; i < 20; i++ {
		candles[i] = Candle{Volume: 100}
	}
	candles[20] = Candle{Volume: 120}

	result := DetectVolumeAnomaly(candles)
	if result.State != "Normal" {
		t.Errorf("expected Normal, got %s (ratio=%f)", result.State, result.Ratio)
	}
	if result.Ratio < 1.1 || result.Ratio > 1.3 {
		t.Errorf("expected ratio ~1.2, got %f", result.Ratio)
	}
}

func TestDetectVolumeAnomaly_Elevated(t *testing.T) {
	candles := make([]Candle, 21)
	for i := 0; i < 20; i++ {
		candles[i] = Candle{Volume: 100}
	}
	candles[20] = Candle{Volume: 180}

	result := DetectVolumeAnomaly(candles)
	if result.State != "Elevated" {
		t.Errorf("expected Elevated, got %s (ratio=%f)", result.State, result.Ratio)
	}
}

func TestDetectVolumeAnomaly_Unusual(t *testing.T) {
	candles := make([]Candle, 21)
	for i := 0; i < 20; i++ {
		candles[i] = Candle{Volume: 100}
	}
	candles[20] = Candle{Volume: 250}

	result := DetectVolumeAnomaly(candles)
	if result.State != "Unusual" {
		t.Errorf("expected Unusual, got %s (ratio=%f)", result.State, result.Ratio)
	}
}

func TestDetectVolumeAnomaly_Spike(t *testing.T) {
	candles := make([]Candle, 21)
	for i := 0; i < 20; i++ {
		candles[i] = Candle{Volume: 100}
	}
	candles[20] = Candle{Volume: 400}

	result := DetectVolumeAnomaly(candles)
	if result.State != "Spike" {
		t.Errorf("expected Spike, got %s (ratio=%f)", result.State, result.Ratio)
	}
}

func TestDetectVolumeAnomaly_EmptyCandles(t *testing.T) {
	result := DetectVolumeAnomaly(nil)
	if result.State != "Normal" {
		t.Errorf("expected Normal for empty candles, got %s", result.State)
	}
}

func TestDetectVolumeAnomaly_SingleCandle(t *testing.T) {
	candles := []Candle{{Volume: 100}}
	result := DetectVolumeAnomaly(candles)
	if result.State != "Normal" {
		t.Errorf("expected Normal for single candle, got %s", result.State)
	}
}
