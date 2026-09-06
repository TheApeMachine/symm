package matrix

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// Transpose changes only matrix addressing. It does not calculate coefficients
// or combine values. Ragged rows are a shape error, not silently padded zeros.
type Transpose[T any] struct {
	core.PrimitiveError
	seed, current core.Primitive
}

func NewTranspose[T any]() *Transpose[T] {
	return &Transpose[T]{seed: transport.NewIO(core.From([][]T{})), current: store.NewRetained(nil)}
}
func (transpose *Transpose[T]) Next(in core.Primitive) core.Primitive {
	result := core.Yield(
		transpose.seed,
		in,
		func(_ [][]T, rows [][]T) [][]T {
			if len(rows) == 0 {
				return [][]T{}
			}
			width := len(rows[0])
			columns := make([][]T, width)
			for column := range columns {
				columns[column] = make([]T, len(rows))
			}
			for row, values := range rows {
				if len(values) != width {
					transpose.Error(core.ErrShape)
					return nil
				}
				for column, value := range values {
					columns[column][row] = value
				}
			}
			return columns
		},
		transpose,
	)
	transport.NewDiscard().Next(transport.NewApply(transpose.current, transport.NewIO(result)))
	return result
}
func (transpose *Transpose[T]) Read() any { return core.To[any](transpose.current) }
