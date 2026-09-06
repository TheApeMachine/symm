package matrix

import (
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/vector"
)

// NewDifference zips equally shaped rows through vector subtraction.
func NewDifference(left, right core.Primitive) core.Primitive {
	return transport.NewPipe(
		transport.NewZip(
			transport.NewPipe(left, transport.NewSpread[[]float64]()),
			transport.NewPipe(right, transport.NewSpread[[]float64]()),
		),
		transport.NewMap(
			transport.NewPipe(
				transport.NewSpread[core.Primitive](),
				transport.NewCollect[[]float64](),
				vector.NewDifference(
					transport.NewPipe(collection.NewAt[[]float64](transport.NewIO(core.From(0.0))), transport.NewSpread[float64]()),
					transport.NewPipe(collection.NewAt[[]float64](transport.NewIO(core.From(1.0))), transport.NewSpread[float64]()),
				),
			),
		),
		transport.NewCollect[[]float64](),
	)
}
