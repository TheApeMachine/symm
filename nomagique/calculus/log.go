package calculus

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Log owns one field operation. What it hands over is the natural logarithm of
each arrival.
*/
type Log[U core.Floating] struct {
	core.Base[U, U]
}

func NewLog[U core.Floating]() *Log[U] {
	return &Log[U]{}
}

func (op *Log[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(U(math.Log(float64(arriving.Read()))))) {
				return
			}
		}
	}
}
