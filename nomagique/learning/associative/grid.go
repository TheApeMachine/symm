package associative

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
)

/*
Grid composes a query-producing primitive with the distributed store and its
output pipeline. Routing can itself be a composition. Metric state stays with
registered owners; the grid does not materialize or copy their values.
*/
type Grid[T core.Ordered[T]] struct {
	*core.PrimitiveError
	pipeline *nomagique.Number
}

func NewGrid[T core.Ordered[T]]() *Grid[T] {
	return &Grid[T]{
		PrimitiveError: core.NewPrimitiveError(),
		pipeline: nomagique.NewNumber(
			store.NewGrid[T](),
		),
	}
}

func (grid *Grid[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return grid.pipeline.Next(in)
}
