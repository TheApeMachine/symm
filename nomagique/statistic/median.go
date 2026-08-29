package statistic

import "github.com/theapemachine/symm/nomagique/types"

/*
Median computes the exact median of populated generic sample slots. An empty
sample set is a valid provisional result with ready zero.
*/
func Median(input *types.Frame) {
	values, count, err := collectSamples(input, "median")

	if err != nil {
		input.Err = err

		return
	}

	result := 0.0
	ready := 0.0

	if count > 0 {
		sortSamples(&values, count)
		middle := count / 2
		result = values[middle]

		if count%2 == 0 {
			result = (values[middle-1] + values[middle]) / 2
		}

		ready = 1
	}

	input.Put(SymbolResult, result)
	input.Put(SymbolReady, ready)
	input.Put(SymbolCount, float64(count))
}
