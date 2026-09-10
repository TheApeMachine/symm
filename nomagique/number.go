package nomagique

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Number threads a run through stages of one type. Heterogeneous composition is
nested Next calls.
*/
func Number[T any](
	in iter.Seq[core.Primitive[T, T]],
	stages ...core.Primitive[T, T],
) iter.Seq[core.Primitive[T, T]] {
	return transport.Pipe(in, stages...)
}
