package temporal

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Governor retains a tail of arrivals whose length is the configured capacity
and hands that tail, as one collection, to a reduction. Until two observations
exist there is nothing to reduce, so the yield is the zero value.
*/
type Governor[U core.Numeric] struct {
	core.Base[U, U]
	capacity  int
	reduction core.Primitive[[]U, U]
	history   []U
}

func NewGovernor[U core.Numeric](capacity int, reduction core.Primitive[[]U, U]) *Governor[U] {
	op := &Governor[U]{capacity: capacity, reduction: reduction}

	if capacity < 1 {
		op.Error(core.ErrShape)
	}

	return op
}

func (op *Governor[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		if op.Error() != nil {
			return
		}

		for arriving := range in {
			op.history = append(op.history, arriving.Read())

			if len(op.history) > op.capacity {
				op.history = append([]U(nil), op.history[len(op.history)-op.capacity:]...)
			}

			if len(op.history) < 2 {
				var zero U

				if !yield(op.Carrier(zero)) {
					return
				}

				continue
			}

			var reduced U

			for out := range op.reduction.Next(transport.Values(op.history)) {
				reduced = out.Read()
			}

			if !yield(op.Carrier(reduced)) {
				return
			}
		}
	}
}
