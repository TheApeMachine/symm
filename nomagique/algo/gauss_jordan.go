package algo

import (
	"fmt"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
GaussJordan owns partial-pivot Gauss-Jordan elimination. Tolerance is the
absolute pivot floor of this solver.

Each arrival is *[2][][]float64{left, right}: left is a square n×n matrix and
right holds its n right-hand-side rows. It yields *[][]float64 whose row [0]
is {defined (1 or 0), rank} and whose rows [1:] are the reduced right-hand
side. A singular system is defined=0 with no solution rows; an empty system is
defined=0 with rank 0. Every yield owns fresh rows, so a held solution is
never overwritten by a later arrival.
*/
type GaussJordan struct {
	*core.PrimitiveError
	tolerance float64
	out       [][]float64
}

func NewGaussJordan(tolerance float64) core.Primitive {
	return &GaussJordan{
		PrimitiveError: core.NewPrimitiveError(),
		tolerance:      tolerance,
	}
}

func (op *GaussJordan) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			system := (*[2][][]float64)(arriving)
			left, right := system[0], system[1]
			rows := len(left)

			for _, row := range left {
				if len(row) != rows {
					op.Error(fmt.Errorf("%w: Gauss-Jordan left-hand side must be square", core.ErrShape))
					return
				}
			}

			if len(right) != rows {
				op.Error(fmt.Errorf("%w: Gauss-Jordan right-hand row count differs from left", core.ErrShape))
				return
			}

			columns := 0

			if rows > 0 {
				columns = len(right[0])
			}

			for _, row := range right {
				if len(row) != columns {
					op.Error(fmt.Errorf("%w: Gauss-Jordan right-hand side is ragged", core.ErrShape))
					return
				}
			}

			a := make([][]float64, rows)
			b := make([][]float64, rows)

			for row := range rows {
				a[row] = append([]float64(nil), left[row]...)
				b[row] = append([]float64(nil), right[row]...)
			}

			rank := 0
			defined := rows > 0

			for col := 0; col < rows; col++ {
				pivotRow := col
				maxVal := math.Abs(a[col][col])

				for r := col + 1; r < rows; r++ {
					if val := math.Abs(a[r][col]); val > maxVal {
						maxVal = val
						pivotRow = r
					}
				}

				if maxVal <= op.tolerance {
					defined = false
					break
				}

				a[col], a[pivotRow] = a[pivotRow], a[col]
				b[col], b[pivotRow] = b[pivotRow], b[col]
				pivot := a[col][col]

				for c := col; c < rows; c++ {
					a[col][c] /= pivot
				}

				for c := 0; c < columns; c++ {
					b[col][c] /= pivot
				}

				for r := 0; r < rows; r++ {
					if r == col {
						continue
					}

					factor := a[r][col]

					for c := col; c < rows; c++ {
						a[r][c] -= factor * a[col][c]
					}

					for c := 0; c < columns; c++ {
						b[r][c] -= factor * b[col][c]
					}
				}

				rank++
			}

			op.out = [][]float64{{0, float64(rank)}}

			if defined {
				op.out[0][0] = 1
				op.out = append(op.out, b...)
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
