package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Transpose changes only matrix addressing. Ragged rows are a shape error, not
silently padded zeros.
*/
type Transpose[T any] struct {
	*core.PrimitiveError

	out [][]T
}

func NewTranspose[T any]() *Transpose[T] {
	return &Transpose[T]{PrimitiveError: core.NewPrimitiveError()}
}

func (transpose *Transpose[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			rows := *(*[][]T)(arriving)

			if len(rows) == 0 {
				transpose.out = [][]T{}

				if !yield(unsafe.Pointer(&transpose.out)) {
					return
				}

				continue
			}

			width := len(rows[0])
			columns := make([][]T, width)

			for column := range columns {
				columns[column] = make([]T, len(rows))
			}

			ok := true

			for row, values := range rows {
				if len(values) != width {
					transpose.Error(core.ErrShape)
					ok = false
					break
				}

				for column, value := range values {
					columns[column][row] = value
				}
			}

			if !ok {
				return
			}

			transpose.out = columns

			if !yield(unsafe.Pointer(&transpose.out)) {
				return
			}
		}
	}
}
