package statistic

import (
	"math"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Maximum returns the greatest populated generic sample. An empty sample set is a
valid provisional result with ready zero.
*/
func Maximum(
	state types.Frame,
	input types.Frame,
) (types.Frame, types.Frame, error) {
	values, count, err := collectSamples(&input, "maximum")

	if err != nil {
		return state, types.Frame{}, err
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

/*
MaxOf returns a primitive that evaluates the maximum over specific named symbols in input.
*/
func MaxOf(symbols ...nomagique.Symbol) nomagique.Primitive {
	return func(
		state types.Frame,
		input types.Frame,
	) (types.Frame, types.Frame, error) {
		result := -math.MaxFloat64
		count := 0

		for _, symbol := range symbols {
			value, found := input.Get(symbol)

			if !found {
				continue
			}

			if value > result {
				result = value
			}

			count++
		}

		ready := 0.0

		if count > 0 {
			ready = 1
		} else {
			result = 0
		}

		output := input
		output.Put(SymbolResult, result)
		output.Put(SymbolReady, ready)
		output.Put(SymbolCount, float64(count))

		return state, output, nil
	}
}
