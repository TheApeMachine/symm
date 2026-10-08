package statistic

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Energy owns the sum of squares of each arrival.
*/
type Energy struct {
	err error
	acc float64
	out float64
}

func NewEnergy() core.Primitive {
	return &Energy{}
}

func (op *Energy) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			val := *(*float64)(arriving)
			op.acc += val * val
			op.out = op.acc

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Energy) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
