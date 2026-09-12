package calculus

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Tanh owns one field operation. What it hands over is the hyperbolic tangent
of each arrival, operating in-place on the wire pointer.
*/
type Tanh struct {
	err error
}

func NewTanh() core.Primitive {
	return &Tanh{}
}

func (op *Tanh) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			*in = math.Tanh(*in)

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Tanh) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
