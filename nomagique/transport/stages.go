package transport

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Stages is a Primitive that threads a run through stages.
*/
type Stages struct {
	*core.PrimitiveError
	stages []core.Primitive
}

func NewStages(stages ...core.Primitive) core.Primitive {
	return &Stages{
		PrimitiveError: core.NewPrimitiveError(),
		stages:         stages,
	}
}

func (op *Stages) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	curr := in

	for _, stage := range op.stages {
		curr = stage.Next(curr)
	}

	return func(yield func(unsafe.Pointer) bool) {
		for out := range curr {
			if !yield(out) {
				return
			}
		}

		for _, stage := range op.stages {
			op.Error(stage.Error())
		}
	}
}
