package arithmetic

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Divide owns one field operation. What arrives is already a pair: the dividend
and divisor as [2]float64. A zero divisor has no quotient, so the primitive
yields no fact rather than an infinity; undefined stays unwritten. Each
arrival maps independently, so the primitive holds no state.
*/
type Divide struct {
	err error
	out float64
}

/*
NewDivide creates the binary division primitive.
*/
func NewDivide() core.Primitive {
	return &Divide{}
}

func (op *Divide) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			pair := (*[2]float64)(arriving)

			if pair[1] == 0 {
				return
			}

			op.out = pair[0] / pair[1]

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Divide) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
