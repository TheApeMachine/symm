package statistic

import (
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
SamplingVarianceInput is specificity debt: matched depth, context length,
support and variance.
*/
type SamplingVarianceInput struct {
	Depth         float64
	ContextLength float64
	Support       float64
	Variance      float64
}

/*
SamplingVariance applies specificity debt, with one observation as the sampling
floor.
*/
type SamplingVariance struct {
	err error
	out float64
}

func NewSamplingVariance() core.Primitive {
	return &SamplingVariance{}
}

func (op *SamplingVariance) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*SamplingVarianceInput)(arriving)

			if input.Depth > input.ContextLength {
				op.err = fmt.Errorf("%w: matched depth exceeds context length", core.ErrDomain)
				return
			}

			floor := input.Support / (1.0 + (input.ContextLength - input.Depth))

			if floor < 1.0 {
				floor = 1.0
			}

			op.out = input.Variance / floor

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *SamplingVariance) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
