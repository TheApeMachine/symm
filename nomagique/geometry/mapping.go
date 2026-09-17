package geometry

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Mapping wraps the spatial transformation pipeline (Inversion, Relaxation, Forest,
Peak, Border). It drives the pipeline over the incoming stream of Edges and yields
the relaxed and segmented edges.
*/
type Mapping[T core.Ordered[T]] struct {
	*core.PrimitiveError
	pipeline core.Primitive
}

func NewMapping[T core.Ordered[T]](stages ...core.Primitive) *Mapping[T] {
	var innerPipeline core.Primitive

	if len(stages) == 1 {
		innerPipeline = stages[0]
	}

	if len(stages) > 1 {
		innerPipeline = nomagique.NewNumber(stages...)
	}

	return &Mapping[T]{
		PrimitiveError: core.NewPrimitiveError(),
		pipeline:       innerPipeline,
	}
}

func (mapping *Mapping[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if mapping.Error() != nil {
			return
		}

		if mapping.pipeline == nil {
			for arriving := range in {
				if !yield(arriving) {
					return
				}
			}
			return
		}

		for out := range mapping.pipeline.Next(in) {
			if !yield(out) {
				return
			}
		}
	}
}
