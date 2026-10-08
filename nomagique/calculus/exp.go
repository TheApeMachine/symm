package calculus

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Exp owns one field operation. What it hands over is the exponential of each
arrival, operating in-place on the wire pointer.
*/
type Exp struct {
	err error
}

func NewExp() core.Primitive {
	return &Exp{}
}

func (op *Exp) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			*in = math.Exp(*in)

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Exp) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
