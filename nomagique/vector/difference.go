package vector

import (
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewDifference subtracts paired members and collects one output vector.
func NewDifference(left, right core.Primitive) core.Primitive {
	return transport.NewPipe(
		transport.NewZip(left, right),
		transport.NewMap(
			equation.NewDifference[float64](
				collection.NewAt[core.Primitive](transport.NewIO(core.From(0.0))),
				collection.NewAt[core.Primitive](transport.NewIO(core.From(1.0))),
			),
		),
		transport.NewCollect[float64](),
	)
}
