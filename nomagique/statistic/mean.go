package statistic

import (
	"github.com/theapemachine/symm/nomagique/types"
)

var SymbolMean = types.MustIntern("mean")

/*
Mean computes the arithmetic center of the populated generic sample slots.
An empty collection is a valid provisional result with ready zero.
*/
func Mean(input *types.Frame) {
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

	input.Put(SymbolMean, mean)
	input.Put(SymbolResult, mean)
	input.Put(SymbolCount, float64(count))
	input.Put(SymbolReady, ready)
}
