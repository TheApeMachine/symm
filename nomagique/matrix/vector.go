package matrix

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
VectorInput is a matrix and the vector it multiplies.
*/
type VectorInput struct {
	Matrix [][]float64
	Vector []float64
}

/*
Vector multiplies a matrix by a vector and retains the vector result.
*/
type Vector struct {
	err error
	out []float64
}

func NewVector() core.Primitive {
	return &Vector{}
}

func (op *Vector) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*VectorInput)(arriving)
			op.out = make([]float64, len(input.Matrix))
			ok := true

			for rowIdx, row := range input.Matrix {
				if len(row) != len(input.Vector) {
					op.Error(core.ErrShape)
					ok = false
					break
				}

				var sum float64

				for colIdx, val := range row {
					sum += val * input.Vector[colIdx]
				}

				op.out[rowIdx] = sum
			}

			if !ok {
				return
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Vector) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
