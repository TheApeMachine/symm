package adaptive

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
standardizerSlots names the running state a Standardizer primitive keeps inside
a frame, all namespaced by the series prefix.
*/
type standardizerSlots struct {
	value     types.Symbol // the observation this step scores
	score     types.Symbol // the standardized z-score output
	mean      types.Symbol // running mean (Welford)
	m2        types.Symbol // running sum of squared deviations (Welford)
	count     types.Symbol // number of observations seen
	precision types.Symbol // predictive precision 1/(centre*spread)
	ready     types.Symbol // 1 once the dispersion is positive
}

func newStandardizerSlots(prefix string) standardizerSlots {
	return standardizerSlots{
		value:     types.MustIntern(joinPrefix(prefix, "value")),
		score:     types.MustIntern(joinPrefix(prefix, "z/value")),
		mean:      types.MustIntern(joinPrefix(prefix, "mean")),
		m2:        types.MustIntern(joinPrefix(prefix, "m2")),
		count:     types.MustIntern(joinPrefix(prefix, "count")),
		precision: types.MustIntern(joinPrefix(prefix, "precision")),
		ready:     types.MustIntern(joinPrefix(prefix, "ready")),
	}
}

/*
Standardizer returns the primitive that scores one observation against its own
prior moments and updates the running Welford statistics in place.

The score is a plain predictive z-score: (sample - priorMean) divided by the
prior predictive scale. The predictive scale is the observed dispersion inflated
by the small-sample uncertainty of estimating both the centre and the spread
from the retained moments, so an early reading is small because the scale it was
measured against is uncertain — not because a counter has not run out. There is
no clamp and no truncation: the score is dimensionless and already self-scaled
by the moments it is measured against.

The input observation is read from the series' value slot and the standardized
score is written to the series' z/value slot. The prefix namespaces every slot;
the empty prefix keeps generic slots. The primitive is constructed once per
series while wiring the pipeline.
*/
func Standardizer(prefix string) types.Primitive {
	slots := newStandardizerSlots(prefix)

	return func(input types.Frame) types.Frame {
		sample, found := input.Get(slots.value)

		if !found {
			input.Err = fmt.Errorf("adaptive: standardizer requires a value")

			return input
		}

		if !utils.IsFinite(sample) {
			input.Err = fmt.Errorf("adaptive: standardizer value must be finite")

			return input
		}

		mean, hasMean := input.Get(slots.mean)
		m2, hasM2 := input.Get(slots.m2)
		count, hasCount := input.Get(slots.count)

		if !hasMean {
			mean = 0
		}

		if !hasM2 {
			m2 = 0
		}

		if !hasCount {
			count = 0
		}

		standardized := 0.0

		if count >= 2 {
			variance := m2 / (count - 1)

			if variance > 0 {
				scale := math.Sqrt(variance) / precisionFor(count)
				standardized = (sample - mean) / scale
			}
		}

		// Advance the Welford moments after scoring, so the sample never
		// dilutes its own score.
		count++
		delta := sample - mean
		mean += delta / count
		delta2 := sample - mean
		m2 += delta * delta2

		input.Put(slots.mean, mean)
		input.Put(slots.m2, m2)
		input.Put(slots.count, count)
		input.Put(slots.score, standardized)
		input.Put(slots.precision, precisionFor(count))

		if count >= 2 && m2 > 0 {
			input.Put(slots.ready, 1)
		} else {
			input.Put(slots.ready, 0)
		}

		return input
	}
}

/*
StandardizerPrecision returns the predictive precision of the moments at the
current count, for callers that want to read the score's confidence without
re-deriving it from the frame.
*/
func StandardizerPrecision(count float64) float64 {
	return precisionFor(count)
}

/*
precisionFor returns the reciprocal of the inflation a predictive scale carries
for having estimated both the centre and the spread from a finite sample. The
two factors are exact statistical identities, not tuning constants:

	centre factor = sqrt(1 + 1/n)          (sampling error of the mean estimate)
	spread factor = 1 + 1/sqrt(2(n-1))     (relative standard error of the
	                                       variance estimate)

It is zero until there are at least two observations, rising toward one as the
moments settle.
*/
func precisionFor(count float64) float64 {
	if count < 2 {
		return 0
	}

	centreFactor := math.Sqrt(1 + 1/count)
	spreadFactor := 1 + 1/math.Sqrt(2*(count-1))

	return 1 / (centreFactor * spreadFactor)
}

func joinPrefix(prefix string, name string) string {
	if prefix == "" {
		return name
	}

	return prefix + "/" + name
}
