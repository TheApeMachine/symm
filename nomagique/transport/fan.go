package transport

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Fan presents one input run to every configured branch and streams what each
branch yields.
*/
type Fan struct {
	err      error
	branches []core.Primitive
}

func NewFan(branches ...core.Primitive) core.Primitive {
	return &Fan{branches: branches}
}

func (op *Fan) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for _, branch := range op.branches {
			for out := range branch.Next(in) {
				if !yield(out) {
					return
				}
			}
		}
	}
}

func (op *Fan) Error(errs ...error) error {
	for _, branch := range op.branches {
		if err := branch.Error(errs...); err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
