package calculus

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Absolute owns one field operation. What it hands over is the absolute value
of each arrival, operating in-place on the wire pointer.
*/
type Absolute struct {
	err error
}

func NewAbsolute() core.Primitive {
	return &Absolute{}
}

func (op *Absolute) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			*in = math.Abs(*in)

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Absolute) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
