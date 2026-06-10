package adaptive

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

/*
Scale returns a positive normalization denominator derived from the running mean.
*/
func (baseline *Baseline) Scale() float64 {
	mean := baseline.moments.Mean()

	if mean > baseline.floor {
		return mean
	}

	return baseline.floor
}

/*
Threshold returns mean + sigma * sqrt(variance + floor^2).
*/
func (baseline *Baseline) Threshold(sigma float64) (float64, bool) {
	if !baseline.Ready() {
		return 0, false
	}

	mean := baseline.moments.Mean()
	spread := math.Sqrt(baseline.Variance() + baseline.floor*baseline.floor)

	return mean + sigma*spread, true
}

/*
ZScore returns (observation - mean) / sqrt(variance + floor^2).
*/
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
