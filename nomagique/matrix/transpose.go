package matrix

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Transpose changes only matrix addressing. Ragged rows are a shape error, not
silently padded zeros.
*/
type Transpose[T any] struct {
	core.Base[[][]T, [][]T]
}

func NewTranspose[T any]() *Transpose[T] {
	return &Transpose[T]{}
}

func (op *Transpose[T]) Next(
	in iter.Seq[core.Primitive[[][]T, [][]T]],
) iter.Seq[core.Primitive[[][]T, [][]T]] {
	return func(yield func(core.Primitive[[][]T, [][]T]) bool) {
		for arriving := range in {
			rows := arriving.Read()

			if len(rows) == 0 {
				if !yield(op.Carrier([][]T{})) {
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
					op.Error(core.ErrShape)
					ok = false
					break
				}

				for column, value := range values {
					columns[column][row] = value
				}
			}

			if !ok {
				continue
			}

			if !yield(op.Carrier(columns)) {
				return
			}
		}
	}
}
