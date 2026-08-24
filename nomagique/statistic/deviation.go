package statistic

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
SymbolDeviation is the relative absolute deviation of the observed value from
the composed baseline.
*/
var SymbolDeviation = types.MustIntern("deviation")

/*
Deviation is a primitive: it reads the current value and the adaptive baseline
and emits their relative absolute deviation. The scale is the baseline's own
magnitude, so a signed residual (fills versus cancellations, depth versus a
negative mean) still has a denominator. A zero baseline has no scale and
reports zero rather than inventing one. The primitive leaves the rest of the
input frame untouched so downstream stages keep whatever context the
composition carried.
*/
func Deviation(input types.Frame) types.Frame {
	value, hasValue := input.Get(types.SampleValue)
	baseline, hasBaseline := input.Get(SymbolBaselineValue)

	if !hasValue || !hasBaseline {
		input.Err = fmt.Errorf(
			"statistic: deviation requires a value and a baseline",
		)

		return input
	}

	input.Put(SymbolDeviation, relativeDeviation(value, baseline))

	return input
}

func relativeDeviation(value float64, baseline float64) float64 {
	scale := math.Abs(baseline)

	if scale == 0 {
		return 0
	}

	return math.Abs(value-baseline) / scale
}

/*
StandardSeparation maps a self-standardizing z-score into a normalized
hypothesis margin. The probability mass inside ±z of a standard normal,
2·Φ(|z|)−1, is zero at the baseline and tends to one as the reading separates
from it. It carries no imposed threshold: the reading's own dispersion is the
denominator, and low magnitude honestly stays near zero while the estimator
has seen little evidence.
*/
func StandardSeparation(zScore float64) float64 {
	accumulated := 0.5 * (1 + math.Erf(math.Abs(zScore)/math.Sqrt2))
	mass := 2*accumulated - 1

	if mass > 0 {
		return mass
	}

	return 0
}
