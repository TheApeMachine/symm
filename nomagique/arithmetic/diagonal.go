package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Diagonal selects row i's member i. An undersized row is a shape error.
*/
type Diagonal struct {
	*core.PrimitiveError

	out []float64
}

func NewDiagonal() *Diagonal {
	return &Diagonal{PrimitiveError: core.NewPrimitiveError()}
}

func (diagonal *Diagonal) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			rows := *(*[][]float64)(arriving)
			diagonal.out = make([]float64, len(rows))
			ok := true

			for index, row := range rows {
				if index >= len(row) {
					diagonal.Error(core.ErrShape)
					ok = false
					break
				}

				diagonal.out[index] = row[index]
			}

			if !ok {
				return
			}

			if !yield(unsafe.Pointer(&diagonal.out)) {
				return
			}
		}
	}
}
