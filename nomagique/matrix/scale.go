package matrix

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
ScaleInput is a matrix and the scalar that multiplies every coefficient.
*/
type ScaleInput struct {
	Values [][]float64
	Factor float64
}

/*
Scale multiplies typed matrix coefficients by one scalar.
*/
type Scale struct {
	err error
	out [][]float64
}

func NewScale() core.Primitive {
	return &Scale{}
}

func (op *Scale) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*ScaleInput)(arriving)
			op.out = make([][]float64, len(input.Values))

			for row, values := range input.Values {
				op.out[row] = make([]float64, len(values))

				for column, value := range values {
					op.out[row][column] = value * input.Factor
				}
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Scale) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
