package equation

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Entropy owns -sum(p log p). Zero mass contributes its limiting value zero;
negative inputs retain the logarithm's undefined-domain result.
*/
type Entropy[U core.Floating] struct {
	core.Base[U, U]
}

func NewEntropy[U core.Floating]() *Entropy[U] {
	op := &Entropy[U]{}
	op.Carrier(0)
	return op
}

func (op *Entropy[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			mass := arriving.Read()
			contribution := U(0)

			if mass != 0 {
				contribution = -mass * U(math.Log(float64(mass)))
			}

			if !yield(op.Carrier(op.Read() + contribution)) {
				return
			}
		}
	}
}
