package calculus

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Sqrt owns one field operation. What it hands over is the square root of each
arrival, operating in-place on the wire pointer.
*/
type Sqrt struct {
	err error
}

func NewSqrt() core.Primitive {
	return &Sqrt{}
}

func (op *Sqrt) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			*in = math.Sqrt(*in)

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Sqrt) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
