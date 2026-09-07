package matrix

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* Difference subtracts equally shaped matrices in typed coefficient storage. */
type Difference struct {
	core.PrimitiveError
	seed    *transport.IO
	current core.Primitive
}

/* NewDifference evaluates each matrix expression once per observation. */
func NewDifference(left, right core.Primitive) core.Primitive {
	return transport.NewPipe(
		transport.NewZip(left, right),
		transport.NewMap(&Difference{seed: transport.NewIO(core.From([][]float64{}))}),
	)
}

/* Next subtracts coefficients without boxing individual values. */
func (difference *Difference) Next(input core.Primitive) core.Primitive {
	result := core.Yield(difference.seed, input,
		func(_ core.Primitive, operands []core.Primitive) core.Primitive {
			return core.Yield(transport.NewIO(operands[0]), transport.NewIO(operands[1]),
				difference.Subtract, difference)
		}, difference)

	if result != nil {
		difference.current = result
	}
	return result
}

/* Subtract preserves both operands and rejects unequal row shapes. */
func (difference *Difference) Subtract(left, right [][]float64) [][]float64 {
	if len(left) != len(right) {
		difference.Error(core.ErrShape)
		return nil
	}
	rows := make([][]float64, len(left))

	for row, values := range left {
		if len(values) != len(right[row]) {
			difference.Error(core.ErrShape)
			return nil
		}
		rows[row] = make([]float64, len(values))

		for column, value := range values {
			rows[row][column] = value - right[row][column]
		}
	}
	return rows
}

func (difference *Difference) Read() any { return core.To[any](difference.current) }
