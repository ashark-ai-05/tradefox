// Package vpin implements the VPIN (Volume-Synchronized Probability of
// Informed Trading) study plugin. VPIN measures buy/sell volume imbalance in
// fixed-size volume buckets and produces a value in the range [0, 1]:
//
//	0 = perfectly balanced trading (equal buy and sell volume)
//	1 = completely imbalanced (all buys or all sells)
//
// The calculation is: VPIN = |buyVolume - sellVolume| / (buyVolume + sellVolume)
//
// Trade volume is accumulated into buckets of a configurable size. When a
// trade causes the bucket to overflow, the bucket is completed and the excess
// volume carries over into the next bucket. Interim VPIN values are emitted
// during accumulation for real-time feedback.
package vpin

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/ashark-ai-05/tradefox/internal/config"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
	"github.com/ashark-ai-05/tradefox/internal/study"
)

const (
	colorGreen = "Green"
	colorWhite = "White"
)

// VPINStudy calculates the Volume-Synchronized Probability of Informed
// Trading by accumulating trade volume into fixed-size buckets and measuring
// the buy/sell imbalance within each bucket.
type VPINStudy struct {
	*study.BaseStudy

	mu                sync.Mutex
	bucketVolumeSize  decimal.Decimal
	currentBucketVol  decimal.Decimal
	currentBuyVolume  decimal.Decimal
	currentSellVolume decimal.Decimal
	lastMarketMidPrice float64

	tradeSubID uint64
	tradeCh    <-chan models.Trade

	bus    *eventbus.Bus
	logger *slog.Logger
}

// New creates a new VPINStudy with the given dependencies and a default
// bucket volume size of 100. The bucket volume size can be overridden via
// SetBucketVolumeSize before calling StartAsync.
func New(bus *eventbus.Bus, settings *config.Manager, logger *slog.Logger) *VPINStudy {
	v := &VPINStudy{
		BaseStudy: study.NewBaseStudy(
			"VPIN", "1.0.0",
			"Volume-Synchronized Probability of Informed Trading",
			"VisualHFT",
			bus, settings, logger,
		),
		bucketVolumeSize: decimal.NewFromFloat(100),
		bus:              bus,
		logger:           logger,
	}

	v.SetTileTitle("VPIN")
	v.SetTileToolTip("Volume-Synchronized Probability of Informed Trading")

	// Set the aggregation hook: keep the latest value (same as C# implementation).
	v.OnDataAggregation = func(existing *models.BaseStudyModel, newItem models.BaseStudyModel, count int) {
		existing.Value = newItem.Value
		existing.MarketMidPrice = newItem.MarketMidPrice
	}

	return v
}

// SetBucketVolumeSize sets the volume threshold for completing a bucket.
// Must be called before StartAsync.
func (v *VPINStudy) SetBucketVolumeSize(size decimal.Decimal) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.bucketVolumeSize = size
}

// BucketVolumeSize returns the current bucket volume threshold.
func (v *VPINStudy) BucketVolumeSize() decimal.Decimal {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.bucketVolumeSize
}

// StartAsync initializes the study, subscribes to the Trades topic on the
// event bus, and starts the trade processing goroutine.
func (v *VPINStudy) StartAsync(ctx context.Context) error {
	if err := v.BaseStudy.StartAsync(ctx); err != nil {
		return err
	}

	v.resetBucket()

	// Subscribe to the trades topic.
	if v.bus != nil {
		v.tradeSubID, v.tradeCh = v.bus.Trades.Subscribe(1024)
		go v.tradeLoop(ctx)
	}

	return nil
}

// StopAsync stops the study and unsubscribes from the Trades topic.
func (v *VPINStudy) StopAsync(ctx context.Context) error {
	if v.bus != nil && v.tradeSubID > 0 {
		v.bus.Trades.Unsubscribe(v.tradeSubID)
		v.tradeSubID = 0
	}

	return v.BaseStudy.StopAsync(ctx)
}

// ProcessTrade processes a single trade, accumulating volume into the current
// bucket and emitting VPIN calculations. This method is exported so that
// tests and direct callers can feed trades without requiring an event bus.
func (v *VPINStudy) ProcessTrade(t models.Trade) {
	if t.IsBuy == nil {
		return
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if v.bucketVolumeSize.IsZero() {
		return
	}

	var bucketOverflow decimal.Decimal

	v.currentBucketVol = v.currentBucketVol.Add(t.Size)

	if v.currentBucketVol.GreaterThan(v.bucketVolumeSize) {
		// Overflow: cap the current bucket and compute the excess.
		bucketOverflow = v.currentBucketVol.Sub(v.bucketVolumeSize)
		v.currentBucketVol = v.bucketVolumeSize

		// Only the portion that fits in this bucket is assigned.
		effectiveSize := t.Size.Sub(bucketOverflow)
		if *t.IsBuy {
			v.currentBuyVolume = v.currentBuyVolume.Add(effectiveSize)
		} else {
			v.currentSellVolume = v.currentSellVolume.Add(effectiveSize)
		}
	} else {
		// No overflow: full trade size goes to this bucket.
		if *t.IsBuy {
			v.currentBuyVolume = v.currentBuyVolume.Add(t.Size)
		} else {
			v.currentSellVolume = v.currentSellVolume.Add(t.Size)
		}
	}

	// Update the last known market mid price.
	v.lastMarketMidPrice = t.MarketMidPrice

	// Emit VPIN for the current (possibly completed) bucket.
	isNewBucket := bucketOverflow.IsPositive()
	v.doCalculation(isNewBucket)

	// If overflow occurred, carry excess volume into the new bucket.
	if isNewBucket {
		if *t.IsBuy {
			v.currentBuyVolume = bucketOverflow
		} else {
			v.currentSellVolume = bucketOverflow
		}
		// The opposite side resets to zero since we only have overflow from one side.
		if *t.IsBuy {
			v.currentSellVolume = decimal.Zero
		} else {
			v.currentBuyVolume = decimal.Zero
		}
		v.currentBucketVol = bucketOverflow
	}
}

// doCalculation computes VPIN from the current bucket state and emits the
// result via AddCalculation. Must be called with v.mu held.
func (v *VPINStudy) doCalculation(isNewBucket bool) {
	totalVolume := v.currentBuyVolume.Add(v.currentSellVolume)

	var vpinValue decimal.Decimal
	if totalVolume.IsPositive() {
		vpinValue = v.currentBuyVolume.Sub(v.currentSellVolume).Abs().Div(totalVolume)
	}

	valueColor := colorWhite
	if isNewBucket {
		valueColor = colorGreen
	}

	item := models.BaseStudyModel{
		Value:                      vpinValue,
		Timestamp:                  time.Now(),
		MarketMidPrice:             v.lastMarketMidPrice,
		ValueColor:                 valueColor,
		AddItemSkippingAggregation: isNewBucket,
	}

	v.AddCalculation(item)

	// Also publish to the Studies topic on the bus for other consumers.
	if v.bus != nil {
		v.bus.Studies.Publish(item)
	}
}

// resetBucket zeroes out all bucket accumulation state.
func (v *VPINStudy) resetBucket() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.currentBucketVol = decimal.Zero
	v.currentBuyVolume = decimal.Zero
	v.currentSellVolume = decimal.Zero
}

// tradeLoop reads trades from the event bus subscription channel and
// processes each one until the context is cancelled or the channel closes.
func (v *VPINStudy) tradeLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case trade, ok := <-v.tradeCh:
			if !ok {
				return
			}
			v.ProcessTrade(trade)
		}
	}
}
