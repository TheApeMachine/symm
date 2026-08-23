package statistic

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolStability = types.MustIntern("stability")
	SymbolRange     = types.MustIntern("range")
)

/*
Stability measures dimensionless clustering around a supplied mean. It emits
one for a collapsed range and otherwise one minus the largest departure from
the mean divided by the collection's own range.
*/
func Stability(
	state types.Frame,
	input types.Frame,
) (types.Frame, types.Frame, error) {
	count := populatedSamples(input)
	output := input

	if count < minimumStabilitySamples {
		output.Put(SymbolStability, 0)
		output.Put(SymbolReady, 0)

		return state, output, nil
	}

	center, found := input.Get(SymbolMean)

	if !found {
		return state, types.Frame{}, fmt.Errorf(
			"statistic: stability requires a mean",
		)
	}

	minimum := math.MaxFloat64
	maximum := -math.MaxFloat64
	maximumDeparture := 0.0

	for index := range types.MaxSamples {
		value, populated := input.Get(types.MustSampleSymbol(index))

		if !populated {
			continue
		}

		minimum = math.Min(minimum, value)
		maximum = math.Max(maximum, value)
		maximumDeparture = math.Max(maximumDeparture, math.Abs(value-center))
	}

	rangeValue := maximum - minimum
	stability := 1.0

	if rangeValue > 0 {
		stability = 1 - maximumDeparture/rangeValue
		stability = math.Max(0, math.Min(1, stability))
	}

	output.Put(SymbolRange, rangeValue)
	output.Put(SymbolStability, stability)
	output.Put(SymbolReady, 1)

	return state, output, nil
}

const minimumStabilitySamples = 2

func populatedSamples(input types.Frame) int {
	count := 0

	for index := range types.MaxSamples {
		if input.Has(types.MustSampleSymbol(index)) {
			count++
		}
	}

	return count
}
