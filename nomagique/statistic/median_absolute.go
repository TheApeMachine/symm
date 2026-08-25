package statistic

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
MedianAbsolute computes the median of the absolute values of the populated
generic sample slots. Like Median, it is a pure primitive: the sample set is
read from the frame's sample/N slots, the absolute values are ranked, and the
result is written to SymbolResult with readiness in SymbolReady and the sample
count in SymbolCount. An empty sample set is a valid provisional result with
ready zero.
*/
func MedianAbsolute(input types.Frame) types.Frame {
	values, count, err := collectSamples(&input, "median-absolute")

	if err != nil {
		input.Err = err

		return input
	}

	for index := 0; index < count; index++ {
		values[index] = math.Abs(values[index])
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

	return input
}

/*
MedianAbsoluteOf returns the median of the absolute values of a raw slice. It
is the slice convenience over MedianAbsolute for callers that do not yet speak
frames: the slice is loaded into the generic sample slots, then the primitive
is evaluated and its result read back. It preserves the external statistic
behavior exactly (finite values only, median of absolute values), but routes
the math through the same primitive every frame consumer uses.
*/
func MedianAbsoluteOf(values []float64) (float64, bool) {
	if len(values) == 0 {
		return 0, false
	}

	frame := types.Frame{}

	for index, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, false
		}

		frame.Put(types.MustSampleSymbol(index), value)
	}

	output := MedianAbsolute(frame)

	if output.Err != nil {
		return 0, false
	}

	result, found := output.Get(SymbolResult)
	ready, hasReady := output.Get(SymbolReady)

	if !found || !hasReady || ready == 0 {
		return 0, false
	}

	return result, true
}
