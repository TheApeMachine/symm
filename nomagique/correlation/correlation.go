package correlation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
CorrelationInput is a covariance and the two energies that normalize it.
*/
type CorrelationInput struct {
	Covariance  float64
	LeftEnergy  float64
	RightEnergy float64
}

/*
Correlation owns covariance / sqrt(left energy * right energy). Empty or
zero-energy normalization is undefined.
*/
type Correlation struct {
	err error
	out float64
}

func NewCorrelation() core.Primitive {
	return &Correlation{}
}

func (op *Correlation) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*CorrelationInput)(arriving)
			scale := math.Sqrt(input.LeftEnergy * input.RightEnergy)
			op.out = input.Covariance / scale

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Correlation) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
