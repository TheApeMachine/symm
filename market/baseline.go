package market

import "math"

/*
Baseline tracks an exponentially weighted mean and variance for scalar telemetry
and exposes z-scores and sigma thresholds without retaining a history buffer.
*/
type Baseline struct {
	moments EWMoments
	floor   float64
	minObs  int
}

func NewBaseline(floor float64, minObs int) *Baseline {
	if minObs < 1 {
		minObs = 1
	}

	return &Baseline{
		floor:  floor,
		minObs: minObs,
	}
}

func (baseline *Baseline) Observe(observation, alpha float64) error {
	return baseline.moments.Update(observation, alpha)
}

func (baseline *Baseline) Ready() bool {
	return baseline.moments.Observations() >= baseline.minObs
}

func (baseline *Baseline) Mean() float64 {
	return baseline.moments.Mean()
}

func (baseline *Baseline) Variance() float64 {
	variance := baseline.moments.VarianceEWMA()

	if variance < 0 {
		return 0
	}

	return variance
}

func (baseline *Baseline) Scale() float64 {
	mean := baseline.moments.Mean()

	if mean > baseline.floor {
		return mean
	}

	return baseline.floor
}

func (baseline *Baseline) Threshold(sigma float64) (float64, bool) {
	if !baseline.Ready() {
		return 0, false
	}

	mean := baseline.moments.Mean()
	spread := math.Sqrt(baseline.Variance() + baseline.floor*baseline.floor)

	return mean + sigma*spread, true
}

func (baseline *Baseline) ZScore(observation, sigma float64) (float64, bool) {
	if !baseline.Ready() {
		return 0, false
	}

	mean := baseline.moments.Mean()
	spread := math.Sqrt(baseline.Variance() + baseline.floor*baseline.floor)

	if spread <= baseline.floor {
		spread = baseline.floor
	}

	if spread <= 0 {
		if observation == mean {
			return 0, true
		}

		return 0, false
	}

	return (observation - mean) / spread, true
}

func (baseline *Baseline) Reset() {
	baseline.moments.Reset()
}

/*
AlphaFromSurprise maps a cross-section surprise index to an EWM blending rate.
Values at or below 1 retain alphaMin; values at or above 2 reach alphaMax.
*/
func AlphaFromSurprise(surpriseIndex, alphaMin, alphaMax float64) float64 {
	if alphaMax <= alphaMin {
		return alphaMin
	}

	safeAlphaMin := alphaMin

	if safeAlphaMin <= 0 {
		safeAlphaMin = 0.001
	}

	excess := surpriseIndex - 1

	if excess <= 0 {
		return safeAlphaMin
	}

	if excess >= 1 {
		return alphaMax
	}

	return safeAlphaMin + (alphaMax-safeAlphaMin)*excess
}
