package equation

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
CorrelationInput is a covariance and the two energies that normalize it.
*/
type CorrelationInput[U core.Floating] struct {
	Covariance  U
	LeftEnergy  U
	RightEnergy U
}

/*
Correlation owns covariance / sqrt(left energy * right energy). Empty or
zero-energy normalization is undefined, not zero or a previous result.
*/
type Correlation[U core.Floating] struct {
	core.Base[CorrelationInput[U], U]
}

func NewCorrelation[U core.Floating]() *Correlation[U] {
	return &Correlation[U]{}
}

func (op *Correlation[U]) Next(
	in iter.Seq[core.Primitive[CorrelationInput[U], CorrelationInput[U]]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			input := arriving.Read()
			scale := U(math.Sqrt(float64(input.LeftEnergy * input.RightEnergy)))

			if !yield(op.Carrier(input.Covariance / scale)) {
				return
			}
		}
	}
}
