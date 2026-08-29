package probability

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

// SymbolDistribution is the slot that receives the softmax-normalized
// distribution over the generic sample slots when present. It is written as the
// winning distribution's length-prefixed vector is not representable in a single
// scalar slot, so Softmax writes the normalized values back into the respective
// sample/N slots and leaves the sum in SymbolDistribution for callers that only
// need a scalar readout.
var SymbolDistribution = types.MustIntern("probability/distribution")

/*
Softmax returns a Primitive that normalizes the populated generic sample slots
into a probability distribution using the numerically stable softmax. Each
sample/N slot is replaced by its normalized probability and the total (which is
exactly 1 for any non-empty finite input) is written to SymbolDistribution.

It is the atomic distribution normalization unit: it knows nothing about what
the samples mean, only that they are finite scores to be exponentiated into a
distribution. A non-finite or empty input sets Err.
*/
func Softmax() types.Primitive {
	return func(input *types.Frame) {
		var values [types.MaxSamples]float64

		count, ok := collectSamples(input, &values)

		if !ok {
			input.Err = fmt.Errorf("probability: softmax requires finite samples")

			return
		}

		if count == 0 {
			input.Err = fmt.Errorf("probability: softmax requires at least one sample")

			return
		}

		maximum := math.Inf(-1)

		for index := 0; index < count; index++ {
			if values[index] > maximum {
				maximum = values[index]
			}
		}

		// probabilities reuses the values array in place: each slot is
		// exponentiated before it is otherwise needed again, so no separate
		// heap slice is required for the intermediate distribution.
		denominator := 0.0

		for index := 0; index < count; index++ {
			values[index] = math.Exp(values[index] - maximum)
			denominator += values[index]
		}

		if denominator <= 0 || math.IsInf(denominator, 0) {
			input.Err = fmt.Errorf("probability: softmax denominator is non-positive")

			return
		}

		// Write the normalized probabilities back into the original sample
		// slots in ascending slot order, so Argmax/EvidenceShare compose.
		slotIndex := 0

		for index := range types.MaxSamples {
			if _, present := input.Get(types.MustSampleSymbol(index)); !present {
				continue
			}

			if slotIndex >= count {
				break
			}

			input.Put(types.MustSampleSymbol(index), values[slotIndex]/denominator)
			slotIndex++
		}

		input.Put(SymbolDistribution, 1.0)
	}
}
