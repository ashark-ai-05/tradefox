package marketresilience

import (
	"math"
	"time"

	"github.com/shopspring/decimal"
	"github.com/ashark-ai-05/tradefox/internal/core/enums"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
)

// ---------------------------------------------------------------------------
// Rolling Window (circular buffer)
// ---------------------------------------------------------------------------

// rollingWindow is a fixed-capacity circular buffer that stores the most
// recent N values. It is used for spread history, trade size history, and
// recovery time history.
type rollingWindow[T float64 | decimal.Decimal] struct {
	buf   []T
	pos   int
	count int
	cap_  int
}

func newRollingWindow[T float64 | decimal.Decimal](capacity int) *rollingWindow[T] {
	return &rollingWindow[T]{
		buf:  make([]T, capacity),
		cap_: capacity,
	}
}

func (rw *rollingWindow[T]) Add(v T) {
	rw.buf[rw.pos] = v
	rw.pos = (rw.pos + 1) % rw.cap_
	if rw.count < rw.cap_ {
		rw.count++
	}
}

func (rw *rollingWindow[T]) Count() int {
	return rw.count
}

// items returns the stored values in insertion order (oldest first).
func (rw *rollingWindow[T]) items() []T {
	if rw.count == 0 {
		return nil
	}
	out := make([]T, rw.count)
	if rw.count < rw.cap_ {
		copy(out, rw.buf[:rw.count])
	} else {
		// Buffer is full; pos points to the oldest element.
		n := copy(out, rw.buf[rw.pos:])
		copy(out[n:], rw.buf[:rw.pos])
	}
	return out
}

// ---------------------------------------------------------------------------
// Decimal rolling window helpers
// ---------------------------------------------------------------------------

func decimalAvg(rw *rollingWindow[decimal.Decimal]) decimal.Decimal {
	if rw.count == 0 {
		return decimal.Zero
	}
	sum := decimal.Zero
	for _, v := range rw.items() {
		sum = sum.Add(v)
	}
	return sum.Div(decimal.NewFromInt(int64(rw.count)))
}

func decimalStdDev(rw *rollingWindow[decimal.Decimal]) decimal.Decimal {
	if rw.count == 0 {
		return decimal.Zero
	}
	avg := decimalAvg(rw)
	sumSq := decimal.Zero
	for _, v := range rw.items() {
		diff := v.Sub(avg)
		sumSq = sumSq.Add(diff.Mul(diff))
	}
	variance, _ := sumSq.Div(decimal.NewFromInt(int64(rw.count))).Float64()
	return decimal.NewFromFloat(math.Sqrt(variance))
}

// ---------------------------------------------------------------------------
// Float64 rolling window helpers
// ---------------------------------------------------------------------------

func float64Avg(rw *rollingWindow[float64]) float64 {
	if rw.count == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range rw.items() {
		sum += v
	}
	return sum / float64(rw.count)
}

// ---------------------------------------------------------------------------
// Timestamped value types (internal state)
// ---------------------------------------------------------------------------

type timestampedValue struct {
	Timestamp time.Time
	Value     decimal.Decimal
}

type timestampedDepth struct {
	Timestamp time.Time
	Value     enums.LOBSide
}

// activeDepthEvent tracks an active LOB depletion event from detection to recovery.
type activeDepthEvent struct {
	t0     time.Time
	tMax   time.Time
	side   enums.LOBSide
	sBase  float64 // spread baseline at t0

	dBaseBid, dBaseAsk     float64 // immediacy depth baselines per side
	dTroughBid, dTroughAsk float64 // worst observed since t0
}

// ---------------------------------------------------------------------------
// OrderBookSnapshot - a lightweight snapshot of an order book for the calculator
// ---------------------------------------------------------------------------

// OrderBookSnapshot captures the fields the calculator needs from an OrderBook
// without holding a reference to the live (mutex-guarded) order book.
type OrderBookSnapshot struct {
	Bids     []models.BookItem
	Asks     []models.BookItem
	Spread   float64
	MidPrice float64
}

// SnapshotFromOrderBook creates an OrderBookSnapshot from a live OrderBook.
// It deep-copies bids and asks, and reads Spread/MidPrice while the lock is held.
func SnapshotFromOrderBook(ob *models.OrderBook) OrderBookSnapshot {
	return OrderBookSnapshot{
		Bids:     ob.Bids(),
		Asks:     ob.Asks(),
		Spread:   ob.Spread(),
		MidPrice: ob.MidPrice(),
	}
}

// ---------------------------------------------------------------------------
// MarketResilienceCalculator
// ---------------------------------------------------------------------------

// Constants matching the C# implementation.
const (
	shockThresholdSigma = 2.0    // 2-sigma outlier detection for trades
	eps                 = 1e-9
	warmupMinSamples    = 200    // avoid cold-start noise
	zKDepth             = 3.0    // robust z-score threshold for depth depletion
	recoveryTarget      = 0.90   // 90% recovery ends event early

	rollingWindowSize = 500 // history size for rolling windows

	// MR score component weights.
	wTrade     = 0.3
	wSpread    = 0.1
	wDepth     = 0.5
	wMagnitude = 0.1
)

// MarketResilienceCalculator implements the core state machine that detects
// trade shocks, spread widening, LOB depth depletion, and recovery to produce
// a Market Resilience score in [0, 1].
//
// It is designed to be called from a single goroutine (the study's processing
// goroutine), so it does not use internal mutexes.
type MarketResilienceCalculator struct {
	maxShockTimeout time.Duration

	// Current prices.
	lastMidPrice *float64
	lastBidPrice *float64
	lastAskPrice *float64
	bidAtHit     *float64
	askAtHit     *float64

	// Rolling windows.
	recentSpreads          *rollingWindow[decimal.Decimal]
	recentTradeSizes       *rollingWindow[decimal.Decimal]
	spreadRecoveryTimes    *rollingWindow[float64]
	depletionRecoveryTimes *rollingWindow[float64]

	// Shock state machine.
	shockTrade     *timestampedValue
	shockSpread    *timestampedValue
	returnedSpread *timestampedValue
	shockDepth     *timestampedDepth
	recoveredDepth *timestampedDepth

	initialHitAtBid *bool

	// Depth depletion / recovery state.
	previousLOB           *OrderBookSnapshot
	lastReportedDepletion enums.LOBSide
	activeDepth           *activeDepthEvent

	// P-squared quantile estimators for robust baselines.
	qSpreadMed  *P2Quantile // median spread
	qBidDMed    *P2Quantile // median immediacy depth (bid)
	qAskDMed    *P2Quantile // median immediacy depth (ask)
	qBidDDevMed *P2Quantile // MAD for bid depth
	qAskDDevMed *P2Quantile // MAD for ask depth

	samplesSpread int
	samplesDepth  int

	// Output.
	CurrentMRScore decimal.Decimal

	// Callback invoked when a new MR score is computed. The study hooks
	// this up to AddCalculation.
	OnScoreCalculated func(score decimal.Decimal, midPrice float64)
}

// NewCalculator creates a new MarketResilienceCalculator with the given
// shock timeout in milliseconds.
func NewCalculator(maxShockTimeoutMs int) *MarketResilienceCalculator {
	return &MarketResilienceCalculator{
		maxShockTimeout: time.Duration(maxShockTimeoutMs) * time.Millisecond,

		recentSpreads:          newRollingWindow[decimal.Decimal](rollingWindowSize),
		recentTradeSizes:       newRollingWindow[decimal.Decimal](rollingWindowSize),
		spreadRecoveryTimes:    newRollingWindow[float64](rollingWindowSize),
		depletionRecoveryTimes: newRollingWindow[float64](rollingWindowSize),

		qSpreadMed:  NewP2Quantile(0.5),
		qBidDMed:    NewP2Quantile(0.5),
		qAskDMed:    NewP2Quantile(0.5),
		qBidDDevMed: NewP2Quantile(0.5),
		qAskDDevMed: NewP2Quantile(0.5),

		CurrentMRScore: decimal.NewFromInt(1), // stable by default
	}
}

// OnTrade processes a new trade. If no shock trade is active and the trade
// is a 2-sigma outlier, it records the trade as a shock event.
func (c *MarketResilienceCalculator) OnTrade(trade models.Trade) {
	now := time.Now()

	if c.shockTrade == nil && c.isLargeTrade(trade.Size) {
		c.shockTrade = &timestampedValue{
			Timestamp: now,
			Value:     trade.Size,
		}
		// Determine if the shock trade hit the bid or ask side.
		if c.lastBidPrice != nil && c.lastAskPrice != nil {
			midPrice := (*c.lastBidPrice + *c.lastAskPrice) / 2
			tradePrice, _ := trade.Price.Float64()
			v := tradePrice <= midPrice
			c.initialHitAtBid = &v
		} else {
			v := false
			c.initialHitAtBid = &v
		}
	} else {
		c.recentTradeSizes.Add(trade.Size)
	}

	c.checkAndCalculateIfShock()
}

// OnOrderBookUpdate processes a new order book snapshot. It tracks spread
// widening/recovery and LOB depth depletion/recovery.
func (c *MarketResilienceCalculator) OnOrderBookUpdate(snap OrderBookSnapshot) {
	now := time.Now()

	if len(snap.Asks) == 0 || len(snap.Bids) == 0 {
		return
	}

	// Check trade shock timeout first.
	if c.shockTrade != nil && now.Sub(c.shockTrade.Timestamp) > c.maxShockTimeout {
		c.shockTrade = nil
		c.bidAtHit = nil
		c.askAtHit = nil
		c.shockSpread = nil
		c.returnedSpread = nil
		c.activeDepth = nil
		c.shockDepth = nil
		c.recoveredDepth = nil

		c.recentSpreads.Add(decimal.NewFromFloat(snap.Spread))
		c.updatePrices(&snap)
		return
	}

	// -- Spread widening / return tracking --
	currentSpread := decimal.NewFromFloat(snap.Spread)

	if c.shockSpread == nil && c.isLargeWideningSpread(currentSpread) {
		if c.shockTrade != nil {
			c.shockSpread = &timestampedValue{
				Timestamp: now,
				Value:     currentSpread,
			}
			if c.lastBidPrice != nil {
				v := *c.lastBidPrice
				c.bidAtHit = &v
			}
			if c.lastAskPrice != nil {
				v := *c.lastAskPrice
				c.askAtHit = &v
			}
		}
	} else if c.shockSpread != nil && c.returnedSpread == nil {
		if now.Sub(c.shockSpread.Timestamp) > c.maxShockTimeout {
			c.bidAtHit = nil
			c.askAtHit = nil
			c.shockSpread = nil
		} else if c.hasSpreadReturnedToMean(currentSpread) {
			c.returnedSpread = &timestampedValue{
				Value:     currentSpread,
				Timestamp: now,
			}
		}
	} else if c.shockSpread != nil && c.returnedSpread != nil {
		if now.Sub(c.shockSpread.Timestamp) > c.maxShockTimeout ||
			absDuration(c.returnedSpread.Timestamp.Sub(c.shockSpread.Timestamp)) > c.maxShockTimeout {
			c.bidAtHit = nil
			c.askAtHit = nil
			c.shockSpread = nil
			c.returnedSpread = nil
		}
	}

	c.recentSpreads.Add(currentSpread)
	c.updatePrices(&snap)

	// -- Depth depletion / recovery tracking --
	depletedState := c.isLOBDepleted(snap)

	if c.shockDepth == nil && depletedState != enums.LOBSideNone {
		if c.shockTrade != nil {
			c.shockDepth = &timestampedDepth{
				Timestamp: now,
				Value:     depletedState,
			}
			c.activateDepthEvent(snap, depletedState)
		}
	} else if c.shockDepth != nil && c.recoveredDepth == nil {
		if now.Sub(c.shockDepth.Timestamp) > c.maxShockTimeout {
			c.activeDepth = nil
			c.shockDepth = nil
		} else {
			recoveredState := c.isLOBRecovered(snap)
			if recoveredState != enums.LOBSideNone {
				c.recoveredDepth = &timestampedDepth{
					Timestamp: now,
					Value:     recoveredState,
				}
			}
		}
	} else if c.shockDepth != nil && c.recoveredDepth != nil {
		if now.Sub(c.shockDepth.Timestamp) > c.maxShockTimeout ||
			absDuration(c.recoveredDepth.Timestamp.Sub(c.shockDepth.Timestamp)) > c.maxShockTimeout {
			c.activeDepth = nil
			c.shockDepth = nil
			c.recoveredDepth = nil
		}
	}

	c.checkAndCalculateIfShock()
}

// ---------------------------------------------------------------------------
// Internal: shock detection
// ---------------------------------------------------------------------------

func (c *MarketResilienceCalculator) isLargeTrade(tradeSize decimal.Decimal) bool {
	if c.recentTradeSizes.Count() < 3 {
		return false
	}
	avg := decimalAvg(c.recentTradeSizes)
	std := decimalStdDev(c.recentTradeSizes)
	if std.IsZero() {
		return false
	}
	threshold := avg.Add(std.Mul(decimal.NewFromFloat(shockThresholdSigma)))
	return tradeSize.GreaterThan(threshold)
}

func (c *MarketResilienceCalculator) isLargeWideningSpread(spreadValue decimal.Decimal) bool {
	if c.recentSpreads.Count() < 3 {
		return false
	}
	avg := decimalAvg(c.recentSpreads)
	std := decimalStdDev(c.recentSpreads)
	if std.IsZero() {
		return false
	}
	threshold := avg.Add(std.Mul(decimal.NewFromFloat(shockThresholdSigma)))
	return spreadValue.GreaterThan(threshold)
}

func (c *MarketResilienceCalculator) hasSpreadReturnedToMean(spreadValue decimal.Decimal) bool {
	avg := decimalAvg(c.recentSpreads)
	return spreadValue.LessThan(avg)
}

// ---------------------------------------------------------------------------
// Internal: LOB depth depletion detection (robust z-score)
// ---------------------------------------------------------------------------

func (c *MarketResilienceCalculator) isLOBDepleted(lob OrderBookSnapshot) enums.LOBSide {
	// 1) Update SPREAD baseline.
	spreadNow := lob.Spread
	if spreadNow <= 0 && c.previousLOB != nil {
		spreadNow = c.previousLOB.Spread
	}
	if spreadNow > 0 {
		c.qSpreadMed.Observe(spreadNow)
		c.samplesSpread++
	}
	spreadBase := spreadNow
	if c.samplesSpread >= warmupMinSamples {
		spreadBase = c.qSpreadMed.Estimate()
	}
	if spreadBase <= 0 {
		spreadBase = 1.0
	}

	// 2) Compute current immediacy-weighted depth per side.
	dBidNow := immedDepthBid(lob, spreadBase)
	dAskNow := immedDepthAsk(lob, spreadBase)

	// 3) Update depth baselines.
	c.qBidDMed.Observe(dBidNow)
	c.qAskDMed.Observe(dAskNow)

	bidMed := c.qBidDMed.Estimate()
	askMed := c.qAskDMed.Estimate()

	// Track absolute deviations for MAD (after P2 initializes at n=5).
	if c.samplesDepth >= 5 {
		c.qBidDDevMed.Observe(math.Abs(dBidNow - bidMed))
		c.qAskDDevMed.Observe(math.Abs(dAskNow - askMed))
	}

	c.samplesDepth++

	if c.samplesDepth < warmupMinSamples {
		c.previousLOB = &lob
		return enums.LOBSideNone
	}

	// 4) Compute robust z-scores using MAD.
	bidMAD := math.Max(c.qBidDDevMed.Estimate(), eps)
	bidZDrop := (bidMed - dBidNow) / bidMAD

	askMAD := math.Max(c.qAskDDevMed.Estimate(), eps)
	askZDrop := (askMed - dAskNow) / askMAD

	// 5) Decide which sides are depleted this tick (edge-triggered).
	var depleted enums.LOBSide

	if bidZDrop >= zKDepth && dBidNow < bidMed {
		depleted |= enums.LOBSideBid
	}
	if askZDrop >= zKDepth && dAskNow < askMed {
		depleted |= enums.LOBSideAsk
	}

	// Edge-trigger: only report NEW depletions.
	newDepletion := depleted & ^c.lastReportedDepletion

	if depleted == enums.LOBSideNone {
		c.lastReportedDepletion = enums.LOBSideNone
	} else if newDepletion != enums.LOBSideNone {
		c.lastReportedDepletion = depleted
	}

	c.previousLOB = &lob
	return newDepletion
}

func (c *MarketResilienceCalculator) activateDepthEvent(lob OrderBookSnapshot, side enums.LOBSide) {
	spreadBase := lob.Spread
	if c.samplesSpread >= warmupMinSamples {
		spreadBase = c.qSpreadMed.Estimate()
	}
	if spreadBase <= 0 {
		spreadBase = math.Max(lob.Spread, 1.0)
	}

	dBidNow := immedDepthBid(lob, spreadBase)
	dAskNow := immedDepthAsk(lob, spreadBase)

	now := time.Now()

	baseBid := dBidNow
	if c.samplesDepth >= warmupMinSamples {
		baseBid = c.qBidDMed.Estimate()
	}
	baseAsk := dAskNow
	if c.samplesDepth >= warmupMinSamples {
		baseAsk = c.qAskDMed.Estimate()
	}

	c.activeDepth = &activeDepthEvent{
		t0:         now,
		tMax:       now.Add(c.maxShockTimeout),
		side:       side,
		sBase:      spreadBase,
		dBaseBid:   baseBid,
		dBaseAsk:   baseAsk,
		dTroughBid: dBidNow,
		dTroughAsk: dAskNow,
	}
}

func (c *MarketResilienceCalculator) isLOBRecovered(lob OrderBookSnapshot) enums.LOBSide {
	if c.activeDepth == nil {
		c.previousLOB = &lob
		return enums.LOBSideNone
	}

	ev := c.activeDepth

	spreadBase := ev.sBase
	if spreadBase <= eps {
		spreadBase = math.Max(lob.Spread, 1.0)
	}

	dBidNow := immedDepthBid(lob, spreadBase)
	dAskNow := immedDepthAsk(lob, spreadBase)

	// Update troughs (worst observed since t0).
	if dBidNow < ev.dTroughBid {
		ev.dTroughBid = dBidNow
	}
	if dAskNow < ev.dTroughAsk {
		ev.dTroughAsk = dAskNow
	}

	// Compute recovery fractions.
	denomBid := math.Max(ev.dBaseBid-ev.dTroughBid, eps)
	denomAsk := math.Max(ev.dBaseAsk-ev.dTroughAsk, eps)

	dRecBid := clamp01((dBidNow - ev.dTroughBid) / denomBid)
	dRecAsk := clamp01((dAskNow - ev.dTroughAsk) / denomAsk)

	var recovered enums.LOBSide
	bidWasDepleted := (ev.side & enums.LOBSideBid) != 0
	askWasDepleted := (ev.side & enums.LOBSideAsk) != 0

	// Check same-side first.
	if bidWasDepleted && dRecBid >= recoveryTarget {
		recovered |= enums.LOBSideBid
	}
	if askWasDepleted && dRecAsk >= recoveryTarget {
		recovered |= enums.LOBSideAsk
	}

	// If none of the depleted sides hit target, allow opposite-side recovery.
	if recovered == enums.LOBSideNone {
		if !bidWasDepleted && dRecBid >= recoveryTarget {
			recovered |= enums.LOBSideBid
		}
		if !askWasDepleted && dRecAsk >= recoveryTarget {
			recovered |= enums.LOBSideAsk
		}
	}

	now := time.Now()
	timedOut := now.After(ev.tMax) || now.Equal(ev.tMax)

	if recovered != enums.LOBSideNone || timedOut {
		if recovered == enums.LOBSideNone && timedOut {
			if dRecBid > dRecAsk {
				recovered = enums.LOBSideBid
			} else if dRecAsk > dRecBid {
				recovered = enums.LOBSideAsk
			} else {
				recovered = enums.LOBSideBoth
			}
		}
		c.activeDepth = nil
		c.previousLOB = &lob
		return recovered
	}

	c.previousLOB = &lob
	return enums.LOBSideNone
}

// ---------------------------------------------------------------------------
// Internal: MR score calculation
// ---------------------------------------------------------------------------

func (c *MarketResilienceCalculator) checkAndCalculateIfShock() {
	completedShocks := 0

	if c.shockSpread != nil && c.returnedSpread != nil {
		completedShocks++
	}
	if c.shockDepth != nil && c.recoveredDepth != nil &&
		c.shockDepth.Value != enums.LOBSideNone && c.recoveredDepth.Value != enums.LOBSideNone {
		completedShocks++
	}

	if completedShocks >= 1 {
		c.triggerMRCalculation()
		c.reset()
	}
}

func (c *MarketResilienceCalculator) triggerMRCalculation() {
	totalWeight := 0.0
	weightedScore := 0.0

	// Component 0: Trade shock severity (30%).
	if c.shockTrade != nil && c.recentTradeSizes.Count() > 0 {
		avg := decimalAvg(c.recentTradeSizes)
		std := decimalStdDev(c.recentTradeSizes)
		if !std.IsZero() {
			tradeZ, _ := c.shockTrade.Value.Sub(avg).Div(std).Float64()
			tradeScore := math.Max(0, 1.0-(tradeZ/6.0))
			weightedScore += wTrade * tradeScore
			totalWeight += wTrade
		}
	}

	// Component 1: Spread recovery (10%).
	if c.shockSpread != nil && c.returnedSpread != nil {
		spreadRecoveryMs := math.Abs(float64(c.returnedSpread.Timestamp.Sub(c.shockSpread.Timestamp).Milliseconds()))
		avgHistMs := spreadRecoveryMs
		if c.spreadRecoveryTimes.Count() > 0 {
			avgHistMs = float64Avg(c.spreadRecoveryTimes)
		}

		spreadScore := avgHistMs / (avgHistMs + spreadRecoveryMs)
		spreadScore = clamp01(spreadScore)

		weightedScore += wSpread * spreadScore
		totalWeight += wSpread

		c.spreadRecoveryTimes.Add(spreadRecoveryMs)
	}

	// Component 2: Depth recovery (50%).
	if c.shockDepth != nil && c.recoveredDepth != nil {
		depRecMs := math.Abs(float64(c.recoveredDepth.Timestamp.Sub(c.shockDepth.Timestamp).Milliseconds()))
		avgHistMs := depRecMs
		if c.depletionRecoveryTimes.Count() > 0 {
			avgHistMs = float64Avg(c.depletionRecoveryTimes)
		}

		depScore := avgHistMs / (avgHistMs + depRecMs)
		depScore = clamp01(depScore)

		weightedScore += wDepth * depScore
		totalWeight += wDepth

		c.depletionRecoveryTimes.Add(depRecMs)
	}

	// Component 3: Spread shock magnitude (10%).
	if c.shockSpread != nil {
		avgHistSpread := c.shockSpread.Value
		if c.recentSpreads.Count() > 0 {
			avgHistSpread = decimalAvg(c.recentSpreads)
		}
		if avgHistSpread.LessThan(decimal.NewFromFloat(0.0001)) {
			avgHistSpread = decimal.NewFromFloat(0.0001)
		}

		magnitudeRatio, _ := c.shockSpread.Value.Div(avgHistSpread).Float64()
		magnitudeScore := clamp01(1.0 / magnitudeRatio)

		weightedScore += wMagnitude * magnitudeScore
		totalWeight += wMagnitude
	}

	// Final score normalization.
	if totalWeight > 0 {
		c.CurrentMRScore = decimal.NewFromFloat(weightedScore / totalWeight)
	} else {
		c.CurrentMRScore = decimal.NewFromInt(1)
	}

	// Invoke callback.
	if c.OnScoreCalculated != nil {
		midPrice := 0.0
		if c.lastMidPrice != nil {
			midPrice = *c.lastMidPrice
		}
		c.OnScoreCalculated(c.CurrentMRScore, midPrice)
	}
}

func (c *MarketResilienceCalculator) reset() {
	c.shockSpread = nil
	c.returnedSpread = nil
	c.shockTrade = nil
	c.shockDepth = nil
	c.recoveredDepth = nil
	c.initialHitAtBid = nil
	c.lastMidPrice = nil
	c.lastBidPrice = nil
	c.lastAskPrice = nil
	c.bidAtHit = nil
	c.askAtHit = nil
	c.activeDepth = nil
}

func (c *MarketResilienceCalculator) updatePrices(snap *OrderBookSnapshot) {
	mid := snap.MidPrice
	c.lastMidPrice = &mid
	if len(snap.Bids) > 0 && snap.Bids[0].Price != nil {
		v := *snap.Bids[0].Price
		c.lastBidPrice = &v
	}
	if len(snap.Asks) > 0 && snap.Asks[0].Price != nil {
		v := *snap.Asks[0].Price
		c.lastAskPrice = &v
	}
}

// ---------------------------------------------------------------------------
// Immediacy-weighted depth calculation
// ---------------------------------------------------------------------------

// immedDepthBid computes the immediacy-weighted depth for the bid side.
// Levels closer to the best price receive higher weight (inverse-square).
func immedDepthBid(lob OrderBookSnapshot, spreadBase float64) float64 {
	if spreadBase <= eps {
		spreadBase = math.Max(lob.Spread, 1.0)
	}
	if len(lob.Bids) == 0 {
		return 0
	}

	best := 0.0
	if lob.Bids[0].Price != nil {
		best = *lob.Bids[0].Price
	}

	acc := 0.0
	for i := range lob.Bids {
		price := 0.0
		if lob.Bids[i].Price != nil {
			price = *lob.Bids[i].Price
		}
		size := 0.0
		if lob.Bids[i].Size != nil {
			size = *lob.Bids[i].Size
		}
		d := (best - price) / spreadBase
		if d < 0 {
			d = 0
		}
		w := invSquareWeight(d)
		acc += size * w
	}
	return acc
}

// immedDepthAsk computes the immediacy-weighted depth for the ask side.
func immedDepthAsk(lob OrderBookSnapshot, spreadBase float64) float64 {
	if spreadBase <= eps {
		spreadBase = math.Max(lob.Spread, 1.0)
	}
	if len(lob.Asks) == 0 {
		return 0
	}

	best := 0.0
	if lob.Asks[0].Price != nil {
		best = *lob.Asks[0].Price
	}

	acc := 0.0
	for i := range lob.Asks {
		price := 0.0
		if lob.Asks[i].Price != nil {
			price = *lob.Asks[i].Price
		}
		size := 0.0
		if lob.Asks[i].Size != nil {
			size = *lob.Asks[i].Size
		}
		d := (price - best) / spreadBase
		if d < 0 {
			d = 0
		}
		w := invSquareWeight(d)
		acc += size * w
	}
	return acc
}

// invSquareWeight returns w = 1 / (1 + d)^2.
func invSquareWeight(d float64) float64 {
	x := 1.0 + d
	return 1.0 / (x * x)
}

// clamp01 clamps v to [0, 1].
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// absDuration returns the absolute value of a time.Duration.
func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
