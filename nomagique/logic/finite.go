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

/*
Holds is the predicate without a streaming run: one value in, one answer out.
Hot-path callers that only need the boolean must not wrap it in Evaluate.
*/
func (op *Finite[U]) Holds(value U) bool {
	number := float64(value)

	return !math.IsNaN(number) && !math.IsInf(number, 0)
}

func (op *Finite[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[bool, bool]] {
	return func(yield func(core.Primitive[bool, bool]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(op.Holds(arriving.Read()))) {
				return
			}
		}
	}
}
