package logic

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Finite owns the finiteness predicate. What it hands over is whether each
arrival is a finite number.
*/
type Finite[U core.Floating] struct {
	core.Base[U, bool]
}

func NewFinite[U core.Floating]() *Finite[U] {
	return &Finite[U]{}
}

func (op *Finite[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[bool, bool]] {
	return func(yield func(core.Primitive[bool, bool]) bool) {
		for arriving := range in {
			value := float64(arriving.Read())

			if !yield(op.Carrier(!math.IsNaN(value) && !math.IsInf(value, 0))) {
				return
			}
		}
	}
}
