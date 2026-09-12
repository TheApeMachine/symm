package arithmetic

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Multiply owns one field operation. Configuration supplies the value a run starts
from; recurrence and delivery remain separate Primitives.
*/
type Multiply struct {
	err error
	acc float64
}

/*
NewMultiply creates a new Multiply primitive with the given initial value.
*/
func NewMultiply(current float64) core.Primitive {
	return &Multiply{
		acc: current,
	}
}

/*
Next folds the incoming run into the value it was configured with and hands
that value over after every arrival, operating in-place on the wire pointer.
*/
func (op *Multiply) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			op.acc *= *in
			*in = op.acc

			if !yield(arriving) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *Multiply) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
