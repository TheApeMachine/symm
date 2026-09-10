package transport

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Pipe threads a run through stages of one type. The data moving between stages
is the iterator each Next returns. Heterogeneous composition is nested Next
calls; a slice cannot hold those.
*/
func Pipe[T any](
	in iter.Seq[core.Primitive[T, T]],
	stages ...core.Primitive[T, T],
) iter.Seq[core.Primitive[T, T]] {
	seq := in

	for _, stage := range stages {
		seq = stage.Next(seq)
	}

	return seq
}
