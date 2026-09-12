package matrix

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
DifferenceInput is a pair of equally shaped matrices.
*/
type DifferenceInput struct {
	Left  [][]float64
	Right [][]float64
}

/*
Difference subtracts equally shaped matrices in typed coefficient storage.
*/
type Difference struct {
	err error
	out [][]float64
}

func NewDifference() core.Primitive {
	return &Difference{}
}

func (op *Difference) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*DifferenceInput)(arriving)

			if len(input.Left) != len(input.Right) {
				op.Error(core.ErrShape)
				return
			}

			op.out = make([][]float64, len(input.Left))
			ok := true

			for row, values := range input.Left {
				if len(values) != len(input.Right[row]) {
					op.Error(core.ErrShape)
					ok = false
					break
				}

				op.out[row] = make([]float64, len(values))

				for column, value := range values {
					op.out[row][column] = value - input.Right[row][column]
				}
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

func (op *Difference) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
