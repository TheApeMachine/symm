package matrix

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
ProductInput is a pair of matrices.
*/
type ProductInput struct {
	Left  [][]float64
	Right [][]float64
}

/*
Product owns matrix multiplication. Coefficients stay in contiguous float64
storage.
*/
type Product struct {
	err error
	out [][]float64
}

func NewProduct() core.Primitive {
	return &Product{}
}

func (op *Product) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*ProductInput)(arriving)
			width := 0

			if len(input.Right) > 0 {
				width = len(input.Right[0])
			}

			ok := true

			for _, row := range input.Right {
				if len(row) != width {
					op.Error(core.ErrShape)
					ok = false
					break
				}
			}

			if !ok {
				return
			}

			for _, row := range input.Left {
				if len(row) != len(input.Right) {
					op.Error(core.ErrShape)
					ok = false
					break
				}
			}

			if !ok {
				return
			}

			op.out = make([][]float64, len(input.Left))
			values := make([]float64, len(input.Left)*width)

			for row, coefficients := range input.Left {
				op.out[row] = values[row*width : (row+1)*width]

				for inner, coefficient := range coefficients {
					for column, value := range input.Right[inner] {
						op.out[row][column] += coefficient * value
					}
				}
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Product) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
