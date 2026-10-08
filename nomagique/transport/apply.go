package transport

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Apply binds a run to a target.
*/
type Apply struct {
	err    error
	target core.Primitive
	bound  iter.Seq[unsafe.Pointer]
}

func NewApply(
	target core.Primitive,
	bound iter.Seq[unsafe.Pointer],
) core.Primitive {
	return &Apply{target: target, bound: bound}
}

func (op *Apply) Next(iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for out := range op.target.Next(op.bound) {
			if !yield(out) {
				return
			}
		}
	}
}

func (op *Apply) Error(errs ...error) error {
	if op.target != nil {
		if err := op.target.Error(errs...); err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
