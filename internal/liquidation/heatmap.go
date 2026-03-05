package liquidation

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// HeatmapData is the complete heatmap response for a symbol.
type HeatmapData struct {
	Symbol       string       `json:"symbol"`
	CurrentPrice float64      `json:"currentPrice"`
	Timestamp    int64        `json:"timestamp"`
	Bands        []HeatmapBand `json:"bands"`
	Stats        HeatmapStats `json:"stats"`
}

// HeatmapStats provides summary analytics about the liquidation landscape.
type HeatmapStats struct {
	TotalAbove           float64     `json:"totalAbove"`
	TotalBelow           float64     `json:"totalBelow"`
	BiggestClusterAbove  HeatmapBand `json:"biggestClusterAbove"`
	BiggestClusterBelow  HeatmapBand `json:"biggestClusterBelow"`
	Asymmetry            float64     `json:"asymmetry"` // above/below ratio
	MagnetDirection      string      `json:"magnetDirection"` // "Up", "Down", "Neutral"
}

// HeatmapSubscriber receives heatmap updates.
type HeatmapSubscriber func(data HeatmapData)

// HeatmapEngine generates and pushes liquidation heatmaps at regular intervals.
type HeatmapEngine struct {
	tracker     *Tracker
	feed        *LiquidationFeed
	logger      *slog.Logger

	mu          sync.RWMutex
	prices      map[string]float64 // latest prices per symbol
	subscribers []HeatmapSubscriber
	latest      map[string]*HeatmapData
}

// NewHeatmapEngine creates a new heatmap engine backed by the given tracker.
func NewHeatmapEngine(tracker *Tracker, logger *slog.Logger) *HeatmapEngine {
	return &HeatmapEngine{
		tracker: tracker,
		feed:    NewLiquidationFeed(),
		logger:  logger,
		prices:  make(map[string]float64),
		latest:  make(map[string]*HeatmapData),
	}
}

// Tracker returns the underlying position tracker.
func (h *HeatmapEngine) Tracker() *Tracker {
	return h.tracker
}

// Feed returns the underlying liquidation feed.
func (h *HeatmapEngine) Feed() *LiquidationFeed {
	return h.feed
}

// UpdatePrice sets the latest known price for a symbol.
func (h *HeatmapEngine) UpdatePrice(symbol string, price float64) {
	h.mu.Lock()
	h.prices[symbol] = price
	h.mu.Unlock()
}

// OnUpdate registers a subscriber for heatmap updates.
func (h *HeatmapEngine) OnUpdate(fn HeatmapSubscriber) {
	h.mu.Lock()
	h.subscribers = append(h.subscribers, fn)
	h.mu.Unlock()
}

// Latest returns the most recently generated heatmap for a symbol, or nil.
func (h *HeatmapEngine) Latest(symbol string) *HeatmapData {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.latest[symbol]
}

// GenerateHeatmap creates a heatmap for a symbol at the given price.
func (h *HeatmapEngine) GenerateHeatmap(symbol string, currentPrice float64, rangePercent float64, numBins int) HeatmapData {
	positions := h.tracker.GetPositionMap(symbol)

	// Fan out all positions into liquidation levels
	var allLevels []LiquidationLevel
	for _, pos := range positions {
		allLevels = append(allLevels, FanOutLiquidations(pos)...)
	}

	// Aggregate into bands
	bands := AggregateLiquidations(allLevels, currentPrice, numBins, rangePercent)
	if bands == nil {
		bands = []HeatmapBand{}
	}

	stats := computeStats(bands, currentPrice)

	return HeatmapData{
		Symbol:       symbol,
		CurrentPrice: currentPrice,
		Timestamp:    time.Now().UnixMilli(),
		Bands:        bands,
		Stats:        stats,
	}
}

// Start begins the periodic heatmap generation loop and position decay.
func (h *HeatmapEngine) Start(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		decayTicker := time.NewTicker(5 * time.Minute)
		defer decayTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.generateAll()
			case <-decayTicker.C:
				h.tracker.DecayOldPositions(72 * time.Hour)
			}
		}
	}()
}

// generateAll builds heatmaps for all symbols with known prices and notifies subscribers.
func (h *HeatmapEngine) generateAll() {
	h.mu.RLock()
	symbols := make(map[string]float64, len(h.prices))
	for s, p := range h.prices {
		symbols[s] = p
	}
	subs := make([]HeatmapSubscriber, len(h.subscribers))
	copy(subs, h.subscribers)
	h.mu.RUnlock()

	for symbol, price := range symbols {
		if price <= 0 {
			continue
		}
		data := h.GenerateHeatmap(symbol, price, 5.0, 200)

		h.mu.Lock()
		h.latest[symbol] = &data
		h.mu.Unlock()

		for _, fn := range subs {
			fn(data)
		}
	}
}

// ProcessLiquidationEvent records a real liquidation event for calibration.
func (h *HeatmapEngine) ProcessLiquidationEvent(event LiquidationEvent) {
	h.feed.Add(event)
}

// computeStats calculates summary statistics from heatmap bands.
func computeStats(bands []HeatmapBand, currentPrice float64) HeatmapStats {
	var stats HeatmapStats
	var bigAboveVol, bigBelowVol float64

	for _, b := range bands {
		midPrice := (b.PriceMin + b.PriceMax) / 2.0
		total := b.LongLiqVolume + b.ShortLiqVolume

		if midPrice > currentPrice {
			stats.TotalAbove += total
			if total > bigAboveVol {
				bigAboveVol = total
				stats.BiggestClusterAbove = b
			}
		} else {
			stats.TotalBelow += total
			if total > bigBelowVol {
				bigBelowVol = total
				stats.BiggestClusterBelow = b
			}
		}
	}

	if stats.TotalBelow > 0 {
		stats.Asymmetry = stats.TotalAbove / stats.TotalBelow
	}

	switch {
	case stats.TotalAbove > stats.TotalBelow*1.2:
		stats.MagnetDirection = "Up"
	case stats.TotalBelow > stats.TotalAbove*1.2:
		stats.MagnetDirection = "Down"
	default:
		stats.MagnetDirection = "Neutral"
	}

	return stats
}
