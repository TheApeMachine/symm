package matrix

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* Scale multiplies typed matrix coefficients by one evaluated scalar. */
type Scale struct {
	core.PrimitiveError
	seed    *transport.IO
	current core.Primitive
}

/* NewScale evaluates both configured operands once per observation. */
func NewScale(values, scale core.Primitive) core.Primitive {
	return transport.NewPipe(
		transport.NewZip(values, scale),
		transport.NewMap(&Scale{seed: transport.NewIO(core.From([][]float64{}))}),
	)
}

/* Next scales complete typed rows without constructing a graph per coefficient. */
func (scale *Scale) Next(input core.Primitive) core.Primitive {
	result := core.Yield(scale.seed, input,
		func(_ core.Primitive, operands []core.Primitive) core.Primitive {
			return core.Yield(transport.NewIO(operands[0]), transport.NewIO(operands[1]),
				func(rows [][]float64, factor float64) [][]float64 {
					scaled := make([][]float64, len(rows))

					for row, values := range rows {
						scaled[row] = make([]float64, len(values))

						for column, value := range values {
							scaled[row][column] = value * factor
						}
					}
					return scaled
				}, scale)
		}, scale)

	if result != nil {
		scale.current = result
	}
	return result
}

func (scale *Scale) Read() any { return core.To[any](scale.current) }
