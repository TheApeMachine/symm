package calculus

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Negate owns one field operation. What it hands over is the additive inverse
of each arrival, operating in-place on the wire pointer.
*/
type Negate struct {
	err error
}

func NewNegate() core.Primitive {
	return &Negate{}
}

func (op *Negate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			*in = -*in

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Negate) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
