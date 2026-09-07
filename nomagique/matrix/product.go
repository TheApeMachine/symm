package matrix

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Product owns matrix multiplication at the typed numeric boundary. Operands are
evaluated once per input and coefficients stay in contiguous float64 storage;
individual scalar products do not construct delivery graphs or boxed values.
*/
type Product struct {
	core.PrimitiveError
	seed    *transport.IO
	current core.Primitive
}

/* NewProduct pairs the configured matrix expressions before multiplying them. */
func NewProduct(left, right core.Primitive) core.Primitive {
	return transport.NewPipe(
		transport.NewZip(left, right),
		transport.NewMap(&Product{seed: transport.NewIO(core.From([][]float64{}))}),
	)
}

/* Next consumes matrix operands through Yield and preserves the completed result. */
func (product *Product) Next(input core.Primitive) core.Primitive {
	result := core.Yield(product.seed, input,
		func(_ core.Primitive, operands []core.Primitive) core.Primitive {
			return core.Yield(transport.NewIO(operands[0]), transport.NewIO(operands[1]),
				product.Multiply, product)
		}, product)

	if result != nil {
		product.current = result
	}

	return result
}

/* Multiply returns a fresh rectangular product without mutating either operand. */
func (product *Product) Multiply(left, right [][]float64) [][]float64 {
	width := 0

	if len(right) > 0 {
		width = len(right[0])
	}

	for _, row := range right {
		if len(row) != width {
			product.Error(core.ErrShape)
			return nil
		}
	}

	for _, row := range left {
		if len(row) != len(right) {
			product.Error(core.ErrShape)
			return nil
		}
	}

	rows := make([][]float64, len(left))
	values := make([]float64, len(left)*width)

	for row, coefficients := range left {
		rows[row] = values[row*width : (row+1)*width]

		for inner, coefficient := range coefficients {
			for column, value := range right[inner] {
				rows[row][column] += coefficient * value
			}
		}
	}

	return rows
}

func (product *Product) Read() any { return core.To[any](product.current) }
