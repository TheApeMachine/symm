package temporal

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Governor retains a tail of arrivals whose length is the configured capacity
and hands that tail, as one collection, to a reduction. Until two observations
exist there is nothing to reduce, so the yield is the zero value.
*/
type Governor struct {
	err       error
	capacity  int
	reduction core.Primitive
	history   []float64
	out       float64
}

func NewGovernor(capacity int, reduction core.Primitive) core.Primitive {
	op := &Governor{capacity: capacity, reduction: reduction}

	if capacity < 1 {
		op.Error(core.ErrShape)
	}

	return op
}

func (op *Governor) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if op.Error() != nil {
			return
		}

		for arriving := range in {
			val := *(*float64)(arriving)
			op.history = append(op.history, val)

			if len(op.history) > op.capacity {
				op.history = append([]float64(nil), op.history[len(op.history)-op.capacity:]...)
			}

			if len(op.history) < 2 {
				op.out = 0

				if !yield(unsafe.Pointer(&op.out)) {
					return
				}

				continue
			}

			if op.reduction != nil {
				redIn := func(yieldRed func(unsafe.Pointer) bool) {
					yieldRed(unsafe.Pointer(&op.history))
				}

				for out := range op.reduction.Next(redIn) {
					op.out = *(*float64)(out)
				}
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Governor) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
