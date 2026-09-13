package arithmetic

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Multiply owns one field operation. What arrives is already a pair: the left
and right operand as [2]float64. Each arrival maps independently, so the
primitive holds no state.
*/
type Multiply struct {
	err error
	out float64
}

/*
NewMultiply creates the binary multiplication primitive.
*/
func NewMultiply() core.Primitive {
	return &Multiply{}
}

func (op *Multiply) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			pair := (*[2]float64)(arriving)
			op.out = pair[0] * pair[1]

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Multiply) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
