// Package marketresilience implements the Market Resilience study, which
// measures how quickly a market recovers after large trade shocks.
package marketresilience

import (
	"math"
	"sort"
)

// P2Quantile implements the P-squared quantile estimator (Jain & Chlamtac, 1985).
// It maintains O(1) space (5 markers) and produces a streaming estimate of
// the pth quantile of an arbitrarily long data stream.
type P2Quantile struct {
	p     float64
	count int

	// The 5 markers: q[i] is the marker height, n[i] is the marker position,
	// np[i] is the desired marker position, dn[i] is the desired increment.
	q  [5]float64
	n  [5]float64
	np [5]float64
	dn [5]float64
}

// NewP2Quantile creates a new P-squared quantile estimator for quantile p,
// where p must be in (0, 1). For median estimation, use p = 0.5.
func NewP2Quantile(p float64) *P2Quantile {
	if p <= 0 || p >= 1 {
		panic("P2Quantile: p must be in (0, 1)")
	}
	return &P2Quantile{p: p}
}

// Count returns the total number of observations added so far.
func (pq *P2Quantile) Count() int {
	return pq.count
}

// Estimate returns the current quantile estimate. During the warmup phase
// (fewer than 5 observations), it returns the last observed value. If no
// observations have been recorded, it returns 0.
func (pq *P2Quantile) Estimate() float64 {
	if pq.count < 5 {
		if pq.count == 0 {
			return 0.0
		}
		// During warmup, return the value at the closest marker to the quantile.
		// Sort the first count values and pick the median-ish one.
		tmp := make([]float64, pq.count)
		copy(tmp, pq.q[:pq.count])
		sort.Float64s(tmp)
		idx := int(float64(pq.count-1) * pq.p)
		return tmp[idx]
	}
	return pq.q[2]
}

// Observe adds a new observation to the estimator. NaN and Inf values are
// silently ignored to maintain robustness.
func (pq *P2Quantile) Observe(x float64) {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return
	}

	if pq.count < 5 {
		pq.q[pq.count] = x
		pq.count++
		if pq.count == 5 {
			// Initialize markers after collecting 5 samples.
			sort.Float64s(pq.q[:])
			for i := 0; i < 5; i++ {
				pq.n[i] = float64(i + 1)
			}
			pq.np[0] = 1
			pq.np[1] = 1 + 2*pq.p
			pq.np[2] = 1 + 4*pq.p
			pq.np[3] = 3 + 2*pq.p
			pq.np[4] = 5
			pq.dn[0] = 0
			pq.dn[1] = pq.p / 2
			pq.dn[2] = pq.p
			pq.dn[3] = (1 + pq.p) / 2
			pq.dn[4] = 1
		}
		return
	}

	// Find cell k and update extreme markers.
	var k int
	switch {
	case x < pq.q[0]:
		pq.q[0] = x
		k = 0
	case x < pq.q[1]:
		k = 0
	case x < pq.q[2]:
		k = 1
	case x < pq.q[3]:
		k = 2
	case x < pq.q[4]:
		k = 3
	default:
		pq.q[4] = x
		k = 3
	}

	// Update positions of markers above k.
	for i := k + 1; i < 5; i++ {
		pq.n[i]++
	}

	// Update desired marker positions.
	for i := 0; i < 5; i++ {
		pq.np[i] += pq.dn[i]
	}

	// Adjust heights of interior markers (i = 1, 2, 3).
	for i := 1; i <= 3; i++ {
		d := pq.np[i] - pq.n[i]

		if (d >= 1 && pq.n[i+1]-pq.n[i] > 1) || (d <= -1 && pq.n[i-1]-pq.n[i] < -1) {
			sign := 1.0
			if d < 0 {
				sign = -1.0
			}

			// P-squared parabolic formula (Jain & Chlamtac, 1985).
			qPar := pq.q[i] + (sign/(pq.n[i+1]-pq.n[i-1]))*
				((pq.n[i]-pq.n[i-1]+sign)*(pq.q[i+1]-pq.q[i])/(pq.n[i+1]-pq.n[i])+
					(pq.n[i+1]-pq.n[i]-sign)*(pq.q[i]-pq.q[i-1])/(pq.n[i]-pq.n[i-1]))

			// Use parabolic if within bounds, else fall back to linear.
			if pq.q[i-1] < qPar && qPar < pq.q[i+1] {
				pq.q[i] = qPar
			} else {
				signInt := int(sign)
				pq.q[i] += sign * (pq.q[i+signInt] - pq.q[i]) / (pq.n[i+signInt] - pq.n[i])
			}
			pq.n[i] += sign
		}
	}

	pq.count++
}
