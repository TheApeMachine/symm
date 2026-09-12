package arithmetic

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Divide owns one field operation. Configuration supplies the value a run starts
from; recurrence and delivery remain separate Primitives.
*/
type Divide struct {
	err error
	acc float64
}

/*
NewDivide creates a new Divide primitive with the given initial value.
*/
func NewDivide(current float64) core.Primitive {
	return &Divide{
		acc: current,
	}
}

/*
Next folds the incoming run into the value it was configured with and hands
that value over after every arrival, operating in-place on the wire pointer.
*/
func (op *Divide) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			op.acc /= *in
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
func (op *Divide) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
