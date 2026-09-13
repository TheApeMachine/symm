package arithmetic

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Add owns one field operation. What arrives is already a pair: the left and
right operand as [2]float64. Each arrival maps independently, so the
primitive holds no state and owns no accumulation.
*/
type Add struct {
	err error
	out float64
}

/*
NewAdd creates the binary addition primitive.
*/
func NewAdd() core.Primitive {
	return &Add{}
}

func (op *Add) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			pair := (*[2]float64)(arriving)
			op.out = pair[0] + pair[1]

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Add) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
