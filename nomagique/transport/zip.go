package transport

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Pair is corresponding values from two runs. Zip does not compare, multiply, or
interpret the pairing.
*/
type Pair[T, U any] struct {
	Left  T
	Right U
}

/*
Zip pairs corresponding yields from two runs. It stops when either run ends.
Neither run is buffered into a collection first.
*/
func Zip[T, U any](
	left iter.Seq[core.Primitive[T, T]],
	right iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[Pair[T, U], Pair[T, U]]] {
	return func(yield func(core.Primitive[Pair[T, U], Pair[T, U]]) bool) {
		next, stop := iter.Pull(right)
		defer stop()

		carrier := &core.Carrier[Pair[T, U]]{}

		for arriving := range left {
			other, ok := next()

			if !ok {
				return
			}

			if !yield(carrier.Carrier(Pair[T, U]{
				Left:  arriving.Read(),
				Right: other.Read(),
			})) {
				return
			}
		}
	}
}
