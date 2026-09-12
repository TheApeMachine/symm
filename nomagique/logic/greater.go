package logic

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Greater owns one ordering relation. Pairing is external: what arrives is already
two values.
*/
type Greater struct {
	err error
	out bool
}

func NewGreater() core.Primitive {
	return &Greater{}
}

func (op *Greater) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*[2]float64)(arriving)
			op.out = in[0] > in[1]

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Greater) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
