package logic

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Not inverts each arrival.
*/
type Not struct {
	err error
	out bool
}

func NewNot() core.Primitive {
	return &Not{}
}

func (op *Not) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*bool)(arriving)
			op.out = !*in

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Not) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
