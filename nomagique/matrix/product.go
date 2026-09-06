package matrix

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/vector"
)

// NewProduct captures right-hand columns once and maps dot products over rows.
// Each operand expression is evaluated once; retained copies, not the original
// producer, supply subsequent row/column passes.
func NewProduct(left, right core.Primitive) core.Primitive {
	columns := store.NewRetained(nil)
	row := store.NewRetained(nil)
	return transport.NewPipe(
		transport.NewFan(
			transport.NewPipe(),
			transport.NewIO(
				transport.NewPipe(right, NewTranspose[float64](), columns, transport.NewDiscard()), left),
		),
		transport.NewSpread[[]float64](),
		transport.NewMap(
			transport.NewPipe(
				row,
				transport.NewApply(
					transport.NewPipe(
						transport.NewSpread[[]float64](),
						transport.NewMap(vector.NewDot(transport.NewApply(transport.NewSpread[float64](), row), transport.NewSpread[float64]())),
						transport.NewCollect[float64](),
					),
					columns,
				),
			),
		),
		transport.NewCollect[[]float64](),
	)
}
