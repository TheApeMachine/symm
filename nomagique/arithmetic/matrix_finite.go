package arithmetic

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
MatrixFinite reports whether every coefficient is a finite number.
*/
type MatrixFinite struct {
	*core.PrimitiveError

	out bool
}

func NewMatrixFinite() *MatrixFinite {
	return &MatrixFinite{PrimitiveError: core.NewPrimitiveError()}
}

func (matrixFinite *MatrixFinite) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			rows := *(*[][]float64)(arriving)
			valid := true

			for _, row := range rows {
				for _, value := range row {
					if math.IsNaN(value) || math.IsInf(value, 0) {
						valid = false
						break
					}
				}

				if !valid {
					break
				}
			}

			matrixFinite.out = valid

			if !yield(unsafe.Pointer(&matrixFinite.out)) {
				return
			}
		}
	}
}
