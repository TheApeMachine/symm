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
