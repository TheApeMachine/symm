package statistic

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Minimum returns the least populated generic sample. An empty sample set is a
valid provisional result with ready zero. It is the exact low-side mirror of
Maximum, so a composed touch can reduce the two sides with one reducer each.
*/
func Minimum(input *types.Frame) {
	values, count, err := collectSamples(input, "minimum")

	if err != nil {
		input.Err = err

		return
	}

	result := 0.0
	ready := 0.0

	if count > 0 {
		result = math.MaxFloat64

		for index := 0; index < count; index++ {
			result = math.Min(result, values[index])
		}

		ready = 1
	}

	input.Put(SymbolResult, result)
	input.Put(SymbolReady, ready)
	input.Put(SymbolCount, float64(count))
}

/*
MinOf returns a primitive that evaluates the minimum over specific named symbols in input.
*/
func MinOf(symbols ...types.Symbol) types.Primitive {
	return func(input *types.Frame) {
		result := math.MaxFloat64
		count := 0

		for _, symbol := range symbols {
			value, found := input.Get(symbol)

			if !found {
				continue
			}

			if value < result {
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

		input.Put(SymbolResult, result)
		input.Put(SymbolReady, ready)
		input.Put(SymbolCount, float64(count))
	}
}
