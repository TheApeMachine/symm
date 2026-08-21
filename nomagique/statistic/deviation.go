package statistic

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique"
)

/*
SymbolDeviation is the relative absolute deviation of the observed value from
the composed baseline.
*/
var SymbolDeviation = nomagique.MustIntern("deviation")

/*
Deviation is a primitive: it reads the current value and the adaptive baseline
and emits their relative absolute deviation. A non-positive baseline has no
scale to deviate against and reports zero rather than inventing one. The
primitive leaves the rest of the input frame untouched so downstream stages
keep whatever context the composition carried.
*/
func Deviation(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	value, hasValue := input.Get(nomagique.SampleValue)
	baseline, hasBaseline := input.Get(SymbolBaselineValue)

	if !hasValue || !hasBaseline {
		return state, nomagique.Frame{}, fmt.Errorf(
			"statistic: deviation requires a value and a baseline",
		)
	}

	output := input
	output.Put(SymbolDeviation, relativeDeviation(value, baseline))

	return state, output, nil
}

func relativeDeviation(value float64, baseline float64) float64 {
	if baseline <= 0 {
		return 0
	}

	return math.Abs(value-baseline) / baseline
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
