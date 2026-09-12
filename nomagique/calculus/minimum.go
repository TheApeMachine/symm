package calculus

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Minimum owns one field operation. Configuration supplies the value a run starts
from. What it hands over is the running minimum after every arrival, operating in-place
on the wire pointer.
*/
type Minimum struct {
	err error
	acc float64
}

func NewMinimum(current float64) core.Primitive {
	return &Minimum{
		acc: current,
	}
}

func (op *Minimum) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			op.acc = math.Min(op.acc, *in)
			*in = op.acc

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Minimum) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
