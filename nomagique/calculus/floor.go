package calculus

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Floor owns one field operation. What it hands over is the greatest integer not
exceeding each arrival.
*/
type Floor[U core.Floating] struct {
	core.Base[U, U]
}

func NewFloor[U core.Floating]() *Floor[U] {
	return &Floor[U]{}
}

func (op *Floor[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(U(math.Floor(float64(arriving.Read()))))) {
				return
			}
		}
	}
}
