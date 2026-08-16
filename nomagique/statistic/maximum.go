package statistic

import (
	"math"

	"github.com/theapemachine/symm/nomagique"
)

/*
Maximum returns the greatest populated generic sample. An empty sample set is a
valid provisional result with ready zero.
*/
func Maximum(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	values, count, err := collectSamples(&input, "maximum")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	result := 0.0
	ready := 0.0

	if count > 0 {
		result = -math.MaxFloat64

		for index := 0; index < count; index++ {
			result = math.Max(result, values[index])
		}

		ready = 1
	}

	output := input
	output.Put(SymbolResult, result)
	output.Put(SymbolReady, ready)
	output.Put(SymbolCount, float64(count))

	return state, output, nil
}
