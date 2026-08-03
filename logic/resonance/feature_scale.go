package resonance

import "math"

/*
featureNormalizer standardizes one feature against its own exact prior history.

Resonance consumes one joint vector spanning measurements emitted by different
signals. Those upstream "normalized" readings are not on one shared scale: some
are raw finite scores, some are ratios, and some are relative deviations. A
predictive-coding residual over that mixed space is only meaningful once each
coordinate has been centered and variance-normalized against its own history.

The current reading is scored against the observations seen before it, so a
transient cannot dilute its own z-score. The sufficient statistics are updated
online with Welford's algorithm, which gives the exact prior mean and sample
variance without introducing retention windows, warmup thresholds, or clipping
heuristics.
*/
type featureNormalizer struct {
	count uint64
	mean  float64
	m2    float64
}

func newFeatureNormalizer() *featureNormalizer {
	return &featureNormalizer{}
}

func (normalizer *featureNormalizer) Standardize(reading float64) float64 {
	standardized := 0.0

	if variance := normalizer.variance(); variance > 0 {
		standardized = (reading - normalizer.mean) / math.Sqrt(variance)
	}

	normalizer.observe(reading)

	return standardized
}

func (normalizer *featureNormalizer) observe(reading float64) {
	normalizer.count++
	delta := reading - normalizer.mean
	normalizer.mean += delta / float64(normalizer.count)
	delta2 := reading - normalizer.mean
	normalizer.m2 += delta * delta2
}

func (normalizer *featureNormalizer) variance() float64 {
	if normalizer.count < 2 {
		return 0
	}

	return normalizer.m2 / float64(normalizer.count-1)
}
