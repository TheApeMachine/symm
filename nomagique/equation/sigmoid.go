package equation

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Sigmoid owns 1 / (1 + exp(-x)).
*/
type Sigmoid[U core.Floating] struct {
	core.Base[U, U]
}

func NewSigmoid[U core.Floating]() *Sigmoid[U] {
	return &Sigmoid[U]{}
}

func (op *Sigmoid[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(1 / (1 + U(math.Exp(float64(-arriving.Read())))))) {
				return
			}
		}
	}
}
