package transport

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
MapReduce is mapper then reducer. There is no second scheduler, queue, or
callback protocol: the reducer's input is the mapper's output iterator.
*/
func MapReduce[T, U, V any](
	mapper core.Primitive[T, U],
	reducer core.Primitive[U, V],
	in iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[V, V]] {
	return reducer.Next(mapper.Next(in))
}
