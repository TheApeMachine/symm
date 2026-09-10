package transport

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Zip2 pairs corresponding yields of the same type as [2]T, which is the shape
comparisons and pairwise field operations consume.
*/
func Zip2[T any](
	left iter.Seq[core.Primitive[T, T]],
	right iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[[2]T, [2]T]] {
	return func(yield func(core.Primitive[[2]T, [2]T]) bool) {
		next, stop := iter.Pull(right)
		defer stop()

		carrier := &core.Carrier[[2]T]{}

		for arriving := range left {
			other, ok := next()

			if !ok {
				return
			}

			if !yield(carrier.Carrier([2]T{arriving.Read(), other.Read()})) {
				return
			}
		}
	}
}
