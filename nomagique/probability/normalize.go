package probability

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Normalize divides each arrival by the run's total.
*/
type Normalize struct {
	err error
	out float64
}

func NewNormalize() core.Primitive {
	return &Normalize{}
}

func (op *Normalize) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values []float64
		var total float64

		for arriving := range in {
			val := *(*float64)(arriving)
			values = append(values, val)
			total += val
		}

		if total == 0 {
			op.err = errors.Join(op.err, core.ErrShape)
			return
		}

		for _, val := range values {
			op.out = val / total

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Normalize) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
