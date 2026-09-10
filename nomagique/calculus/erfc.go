package calculus

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Erfc owns one field operation. What it hands over is the complementary error
function of each arrival.
*/
type Erfc[U core.Floating] struct {
	core.Base[U, U]
}

func NewErfc[U core.Floating]() *Erfc[U] {
	return &Erfc[U]{}
}

func (op *Erfc[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(U(math.Erfc(float64(arriving.Read()))))) {
				return
			}
		}
	}
}
