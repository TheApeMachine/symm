package calculus

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Sign owns one field operation. What it hands over is the unit sign of each
arrival. Zero keeps its own value, including a signed zero.
*/
type Sign[U core.Floating] struct {
	core.Base[U, U]
}

func NewSign[U core.Floating]() *Sign[U] {
	return &Sign[U]{}
}

func (op *Sign[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			value := arriving.Read()
			signed := value

			if value != 0 {
				signed = U(math.Copysign(1, float64(value)))
			}

			if !yield(op.Carrier(signed)) {
				return
			}
		}
	}
}
