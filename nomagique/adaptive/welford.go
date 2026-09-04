package adaptive

import (
	"math"
)

// MinimumSampleCountForVariance is the degrees of freedom threshold (n >= 2).
const MinimumSampleCountForVariance = 2.0

/*
WelfordEngine provides single-pass, numerically stable running sample variance
and mean updates according to B. P. Welford (1962, Technometrics).
Computes sample variance with n - 1 degrees of freedom (unbiased Bessel correction).
Zero heap allocations.
*/
type WelfordEngine struct {
	count float64
	mean  float64
	m2    float64
}

// Update incorporates a new sample into the running moments and returns (mean, stdDev).
func (welford *WelfordEngine) Update(value float64) (float64, float64) {
	welford.count++
	delta := value - welford.mean
	welford.mean += delta / welford.count
	delta2 := value - welford.mean
	welford.m2 += delta * delta2

	if welford.count < MinimumSampleCountForVariance {
		return welford.mean, 0
	}

	variance := welford.m2 / (welford.count - 1.0)

	return welford.mean, math.Sqrt(variance)
}

func (welford *WelfordEngine) Mean() float64 {
	return welford.mean
}

func (welford *WelfordEngine) Dispersion() float64 {
	if welford.count < MinimumSampleCountForVariance {
		return 0
	}

	return math.Sqrt(welford.m2 / (welford.count - 1.0))
}

func (welford *WelfordEngine) Variance() float64 {
	if welford.count < MinimumSampleCountForVariance {
		return 0
	}

	return welford.m2 / (welford.count - 1.0)
}

func (welford *WelfordEngine) Count() float64 {
	return welford.count
}

/*
Shed scales the accumulated sample mass by retain, holding the mean fixed.

An estimator that never forgets converges on the pooled moments of every
regime it has ever seen: its dispersion grows to span the differences between
regimes, and its mean stops moving because each new sample carries weight
1/count. Both make a departure from the current regime unmeasurable, which is
the opposite of what a baseline exists to do.

Shedding scales count and the sum of squared deviations by the same factor, so
the mean and the dispersion are both preserved at the moment of the call while
the support behind them collapses. Nothing is discarded discontinuously: the
estimator keeps its current best statement of the level and simply becomes
willing to be moved off it again.
*/
func (welford *WelfordEngine) Shed(retain float64) {
	if retain >= 1 || retain <= 0 || welford.count <= MinimumSampleCountForVariance {
		return
	}

	shed := welford.count * retain

	if shed < MinimumSampleCountForVariance {
		shed = MinimumSampleCountForVariance
	}

	// Dispersion is the Bessel-corrected sample deviation sqrt(m2/(n-1)), so
	// holding it fixed across the shed means scaling m2 by the ratio of the
	// corrected degrees of freedom, not by the ratio of the raw counts.
	welford.m2 *= (shed - 1) / (welford.count - 1)
	welford.count = shed
}
