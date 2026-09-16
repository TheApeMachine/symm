package associative

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Grid composes owner registration and raw-input execution against one distributed
store. Each input is a borrowed map[[2]string]T, addressed by entity and raw key;
each output is the completed store.Grid[T] index of owner-resident metrics.
Registration queries execute once, before any owner receives market input.
*/
type Grid[T any] struct {
	*core.PrimitiveError
	pipeline *nomagique.Number
}

func NewGrid[T any](registrations ...core.Primitive) *Grid[T] {
	resident := store.NewGrid[T]()
	return &Grid[T]{
		PrimitiveError: core.NewPrimitiveError(),
		pipeline: nomagique.NewNumber(
			transport.NewOnce(nomagique.NewNumber(
				transport.NewFan(registrations...),
				resident,
			)),
			store.NewQuery[T](nil, data.ActionExecute),
			resident,
		),
	}
}

func (grid *Grid[T]) Next(input iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for output := range grid.pipeline.Next(input) {
			if !yield(output) {
				break
			}
		}

		if err := grid.pipeline.Error(); err != nil {
			grid.Error(err)
		}
	}
}
