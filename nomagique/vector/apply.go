package vector

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Apply aligns one arrival with each configured operation and forwards that
operation's complete output. Operations retain their identity between runs, so
each coordinate may own an independent recurrence.
*/
type Apply struct {
	err        error
	operations []core.Primitive
}

func NewApply(operations ...core.Primitive) core.Primitive {
	return &Apply{operations: operations}
}

func (op *Apply) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		index := 0

		for arriving := range in {
			if index >= len(op.operations) {
				op.Error(core.ErrShape)
				return
			}

			once := func(y func(unsafe.Pointer) bool) {
				y(arriving)
			}

			for out := range op.operations[index].Next(once) {
				if !yield(out) {
					return
				}
			}

			op.Error(op.operations[index].Error())
			index++
		}

		if index != len(op.operations) {
			op.Error(core.ErrShape)
		}
	}
}

func (op *Apply) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}
	for _, operation := range op.operations {
		if err := operation.Error(); err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
