package logic

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Equal owns one equality relation. Pairing is external: what arrives is already
two values.
*/
type Equal struct {
	err error
	out bool
}

func NewEqual() core.Primitive {
	return &Equal{}
}

func (op *Equal) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*[2]float64)(arriving)
			op.out = in[0] == in[1]

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Equal) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
