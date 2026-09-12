package matrix

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Column arranges a scalar run as an n-by-one matrix.
*/
type Column struct {
	err error
	out [][]float64
}

func NewColumn() core.Primitive {
	return &Column{}
}

func (op *Column) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values []float64

		for arriving := range in {
			values = append(values, *(*float64)(arriving))
		}

		op.out = make([][]float64, len(values))

		for index, value := range values {
			op.out[index] = []float64{value}
		}

		if !yield(unsafe.Pointer(&op.out)) {
			return
		}
	}
}

func (op *Column) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
