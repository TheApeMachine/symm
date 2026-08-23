package statistic

import (
	"github.com/theapemachine/symm/nomagique/types"
)

var SymbolMean = types.MustIntern("mean")

/*
Mean computes the arithmetic center of the populated generic sample slots.
An empty collection is a valid provisional result with ready zero.
*/
func Mean(
	state types.Frame,
	input types.Frame,
) (types.Frame, types.Frame, error) {
	mean := 0.0
	count := 0

	for index := range types.MaxSamples {
		value, found := input.Get(types.MustSampleSymbol(index))

		if !found {
			continue
		}

		count++
		mean += (value - mean) / float64(count)
	}

	ready := 0.0

	if count > 0 {
		ready = 1
	}

	output := input
	output.Put(SymbolMean, mean)
	output.Put(SymbolResult, mean)
	output.Put(SymbolCount, float64(count))
	output.Put(SymbolReady, ready)

	return state, output, nil
}
