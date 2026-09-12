package statistic

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Count counts delivered objects, regardless of their payload.
*/
type Count struct {
	err error
	out float64
}

func NewCount() core.Primitive {
	return &Count{}
}

func (op *Count) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for range in {
			op.out++

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Count) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
