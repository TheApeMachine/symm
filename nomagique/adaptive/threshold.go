package adaptive

import (
	"math"
)

/*
ThresholdType defines the adaptive filter threshold strategy.
*/
type ThresholdType int

const (
	WELFORD ThresholdType = iota
	MAD
	CHEBYSHEV
)

// NormalConsistencyFactor = 1 / (sqrt(2) * erfInv(0.5)) ≈ 1.482602218505602.
// Universal scale factor to make MAD an asymptotically unbiased estimator of standard deviation under Gaussian normality.
const NormalConsistencyFactor = 1.482602218505602

// MinimumDegreesOfFreedom is the minimum sample count required to compute variance (n >= 2).
const MinimumDegreesOfFreedom = 2

/*
Threshold self-calibrates to running dispersion and statistical moments online
rather than using fixed sigma multipliers.
*/
type Threshold struct {
	Type ThresholdType

	engine WelfordEngine
	count  int
}

// Compute returns the dynamic threshold value for the current observation.
func (threshold *Threshold) Compute(value float64) float64 {
	threshold.count++
	_, stdDev := threshold.engine.Update(value)

	if stdDev <= 0 {
		return 1.0
	}

	switch threshold.Type {
	case WELFORD:
		// Predictive scale inflation factor for small sample size: sqrt(1 + 1/n)
		inflation := 1.0

		if threshold.count >= MinimumDegreesOfFreedom {
			inflation = math.Sqrt(1.0 + 1.0/float64(threshold.count))
		}

		return stdDev * inflation

	case MAD:
		// Asymptotically unbiased Gaussian normal consistency
		return stdDev * NormalConsistencyFactor

	case CHEBYSHEV:
		// Distribution-free Chebyshev confidence frontier:
		// P(|X - mu| >= k * sigma) <= 1/k^2.
		// For confidence tail significance alpha = 1/count, k = sqrt(count).
		k := math.Sqrt(float64(threshold.count))

		if k < 1.0 {
			k = 1.0
		}

		return stdDev * k

	default:
		return stdDev
	}
}
