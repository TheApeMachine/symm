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
