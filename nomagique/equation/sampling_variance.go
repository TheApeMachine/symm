package equation

import (
	"fmt"
	"iter"

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
func SamplingVariance(depth, contextLength, support, variance float64) (float64, error) {
	if depth > contextLength {
		return 0, fmt.Errorf("%w: matched depth exceeds context length", core.ErrDomain)
	}

	floor := support / (1 + (contextLength - depth))

	if floor < 1 {
		floor = 1
	}

	return variance / floor, nil
}

/*
SamplingVarianceOp binds that equation to the Primitive contract.
*/
type SamplingVarianceOp struct {
	core.Base[SamplingVarianceInput, float64]
}

func NewSamplingVariance() *SamplingVarianceOp {
	return &SamplingVarianceOp{}
}

func (op *SamplingVarianceOp) Next(
	in iter.Seq[core.Primitive[SamplingVarianceInput, SamplingVarianceInput]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for arriving := range in {
			input := arriving.Read()
			value, err := SamplingVariance(input.Depth, input.ContextLength, input.Support, input.Variance)

			if err != nil {
				op.Error(err)
				continue
			}

			if !yield(op.Carrier(value)) {
				return
			}
		}
	}
}
