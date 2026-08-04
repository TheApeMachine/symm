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
variance.

A sample variance is an estimate carrying its own error, and for a handful of
observations that error is the whole quantity: the relative standard error of a
variance over n samples is about sqrt(2/(n-1)), which is 100% at three samples.
Dividing by a deviation drawn from two readings that happened to land close
together does not produce a large z-score, it produces an arbitrary one — the
market's first two ticks decide the scale of every reading after them. Measured
end to end, that put surprise at 5.9e12 against an expected order of one, which
saturates every tanh downstream and trains the manifold on the accident rather
than on the market.

So a scale is used only once enough observations exist for it to mean anything,
and a standardized reading is bounded by how far out an observation can be
before it is a regime change rather than a deviation. Both are stated as
constants below with the reasoning that fixes them.
*/
type featureNormalizer struct {
	count uint64
	mean  float64
	m2    float64
}

const (
	/*
		featureWarmup is how many observations a feature needs before its own
		variance is trusted as a scale. At thirty-two the variance estimate
		carries roughly a quarter of its own magnitude as error, which is the
		point where dividing by it describes the feature rather than the sample.
	*/
	featureWarmup = 32

	/*
		featureDeviations bounds a standardized reading. Beyond eight sigma an
		observation is no longer a deviation within the distribution being
		tracked, it is evidence the distribution moved, and the normalizer has
		no way to tell those apart from inside. Bounding keeps one such reading
		from setting the manifold's residual by itself; the running statistics
		still absorb it in full, so a genuine regime shift moves the scale
		within a few dozen observations.
	*/
	featureDeviations = 8.0
)

func newFeatureNormalizer() *featureNormalizer {
	return &featureNormalizer{}
}

func (normalizer *featureNormalizer) Standardize(reading float64) float64 {
	standardized := 0.0

	/*
		Zero until the feature has a scale worth dividing by. A coordinate that
		reads as no deviation contributes nothing to the residual, which is the
		honest reading while the only thing known about it is its own first few
		samples.
	*/
	if normalizer.count >= featureWarmup {
		if deviation := math.Sqrt(normalizer.variance()); deviation > 0 {
			standardized = math.Max(
				-featureDeviations,
				math.Min(featureDeviations, (reading-normalizer.mean)/deviation),
			)
		}
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
