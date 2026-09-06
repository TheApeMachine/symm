package matrix

import (
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewDiagonal selects row i's member i. An undersized row is a shape error.
func NewDiagonal() core.Primitive {
	row := store.NewRetained(nil)
	return transport.NewPipe(
		transport.NewSpread[[]float64](),
		transport.NewEnumerate(),
		transport.NewMap(
			transport.NewPipe(
				row,
				collection.NewAt[core.Primitive](transport.NewIO(core.From(1.0))),
				collection.NewAt[float64](transport.NewApply(
					collection.NewAt[core.Primitive](transport.NewIO(core.From(0.0))), row)),
			),
		),
		transport.NewCollect[float64](),
	)
}
