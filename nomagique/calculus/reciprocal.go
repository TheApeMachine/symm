package calculus

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Reciprocal owns one field operation. What it hands over is the multiplicative
inverse of each arrival, operating in-place on the wire pointer.
*/
type Reciprocal struct {
	err error
}

func NewReciprocal() core.Primitive {
	return &Reciprocal{}
}

func (op *Reciprocal) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			*in = 1.0 / *in

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Reciprocal) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
