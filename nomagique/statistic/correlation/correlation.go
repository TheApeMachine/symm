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
	*core.PrimitiveError

	out float64
}

func NewCorrelation() *Correlation {
	return &Correlation{PrimitiveError: core.NewPrimitiveError()}
}

func (correlation *Correlation) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*CorrelationInput)(arriving)
			scale := math.Sqrt(input.LeftEnergy * input.RightEnergy)
			correlation.out = input.Covariance / scale

			if !yield(unsafe.Pointer(&correlation.out)) {
				return
			}
		}
	}
}
