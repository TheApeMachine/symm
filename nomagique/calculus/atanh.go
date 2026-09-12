package calculus

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Atanh owns one field operation. What it hands over is the inverse hyperbolic
tangent of each arrival, operating in-place on the wire pointer.
*/
type Atanh struct {
	err error
}

func NewAtanh() core.Primitive {
	return &Atanh{}
}

func (op *Atanh) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			*in = math.Atanh(*in)

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Atanh) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
