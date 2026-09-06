package matrix

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewIdentity enumerates matching row/column coordinates. Identity is topology
// plus equality, not a second implementation of a numeric matrix kernel.
func NewIdentity() core.Primitive {
	size := store.NewRetained(core.From(0.0))
	row := store.NewRetained(core.From(0.0))
	return transport.NewPipe(
		size,
		transport.NewApply(
			transport.NewPipe(
				transport.NewMap(
					transport.NewPipe(
						row,
						transport.NewApply(
							transport.NewPipe(
								transport.NewMap(
									logic.NewGate(
										equation.NewEqual[float64](transport.NewPipe(), transport.NewApply(row, nil)),
										store.NewConstant(core.From(1.0)),
										store.NewConstant(core.From(0.0)),
									),
								),
								transport.NewCollect[float64](),
							),
							transport.NewRange(size),
						),
					),
				),
				transport.NewCollect[[]float64](),
			),
			transport.NewRange(size),
		),
	)
}
