package correlation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Correlation owns covariance / sqrt(left energy * right energy). Each arrival is
[3]float64{covariance, leftEnergy, rightEnergy}; it yields *float64. Empty or
zero-energy normalization is undefined (NaN/Inf from the division).
*/
type Correlation struct {
	*core.PrimitiveError
	out float64
}

func NewCorrelation() core.Primitive {
	return &Correlation{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Correlation) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			input := *(*[3]float64)(arriving)
			op.out = input[0] / math.Sqrt(input[1]*input[2])

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
