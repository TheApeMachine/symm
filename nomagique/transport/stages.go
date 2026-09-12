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
	err    error
	stages []core.Primitive
}

func NewStages(stages ...core.Primitive) core.Primitive {
	return &Stages{stages: stages}
}

func (op *Stages) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	curr := in

	for _, stage := range op.stages {
		curr = stage.Next(curr)
	}

	return curr
}

func (op *Stages) Error(errs ...error) error {
	for _, stage := range op.stages {
		if err := stage.Error(errs...); err != nil {
			op.err = err
		}
	}

	return op.err
}
