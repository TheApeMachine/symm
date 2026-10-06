package statistic

import (
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
SamplingVariance applies specificity debt, with one observation as the sampling
floor. Each arrival is *[4]float64 {depth, context length, support, variance}.
*/
type SamplingVariance struct {
	*core.PrimitiveError
	out float64
}

func NewSamplingVariance() *SamplingVariance {
	return &SamplingVariance{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *SamplingVariance) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			input := (*[4]float64)(arriving)
			depth, contextLength, support, variance := input[0], input[1], input[2], input[3]

			if depth > contextLength {
				op.Error(fmt.Errorf("%w: matched depth exceeds context length", core.ErrDomain))
				return
			}

			floor := support / (1.0 + (contextLength - depth))

			if floor < 1.0 {
				floor = 1.0
			}

			op.out = variance / floor

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
