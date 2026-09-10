package equation

import (
	"iter"
	"time"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Elapsed subtracts int64 nanoseconds before conversion to seconds so epoch
magnitude cannot erase a small interval by cancellation.
*/
type Elapsed[T any] struct {
	core.Base[T, float64]
	from    core.Primitive[T, int64]
	through core.Primitive[T, int64]
}

func NewElapsed[T any](from, through core.Primitive[T, int64]) *Elapsed[T] {
	return &Elapsed[T]{from: from, through: through}
}

func (op *Elapsed[T]) Next(
	in iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for pair := range transport.Zip(op.through.Next(in), op.from.Next(in)) {
			sides := pair.Read()

			if !yield(op.Carrier(float64(sides.Left-sides.Right) / float64(time.Second))) {
				return
			}
		}
	}
}
