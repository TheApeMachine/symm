package calculus

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
BoundRecord is a value and the interval that may replace it.
*/
type BoundRecord struct {
	Value float64
	Lower float64
	Upper float64
}

/*
Bound selects lower, value, or upper.
*/
type Bound struct {
	err error
	out float64
}

func NewBound() core.Primitive {
	return &Bound{}
}

func (op *Bound) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			record := *(*BoundRecord)(arriving)
			val := record.Value

			if val < record.Lower {
				val = record.Lower
			}

			if val > record.Upper {
				val = record.Upper
			}

			op.out = val

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Bound) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
