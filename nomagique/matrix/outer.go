package matrix

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
OuterInput is two vectors lifted to a column and a row.
*/
type OuterInput struct {
	Left  []float64
	Right []float64
}

/*
Outer owns the outer product left ⊗ right.
*/
type Outer struct {
	err error
	out [][]float64
}

func NewOuter() core.Primitive {
	return &Outer{}
}

func (op *Outer) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*OuterInput)(arriving)
			op.out = make([][]float64, len(input.Left))
			values := make([]float64, len(input.Left)*len(input.Right))
			width := len(input.Right)

			for row, lval := range input.Left {
				op.out[row] = values[row*width : (row+1)*width]

				for col, rval := range input.Right {
					op.out[row][col] = lval * rval
				}
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Outer) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
