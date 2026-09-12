package calculus

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Square owns one field operation. What it hands over is the square of each
arrival, operating in-place on the wire pointer.
*/
type Square struct {
	err error
}

func NewSquare() core.Primitive {
	return &Square{}
}

func (op *Square) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			*in = *in * *in

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Square) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
