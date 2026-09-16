package equation

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
	*core.PrimitiveError

	out float64
}

/*
NewSamplingVariance creates the specificity-debt equation Primitive.
*/
func NewSamplingVariance() *SamplingVarianceOp {
	return &SamplingVarianceOp{PrimitiveError: core.NewPrimitiveError()}
}

/*
Next receives *SamplingVarianceInput payloads and yields a *float64 sampling
variance for each.
*/
func (samplingVarianceOp *SamplingVarianceOp) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*SamplingVarianceInput)(arriving)
			value, err := SamplingVariance(input.Depth, input.ContextLength, input.Support, input.Variance)

			if err != nil {
				samplingVarianceOp.Error(err)
				return
			}

			samplingVarianceOp.out = value

			if !yield(unsafe.Pointer(&samplingVarianceOp.out)) {
				return
			}
		}
	}
}
