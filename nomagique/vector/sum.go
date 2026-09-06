package vector

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewSum adds paired scalar streams and collects their vector.
func NewSum(left, right core.Primitive) core.Primitive {
	return transport.NewPipe(
		transport.NewZip(left, right),
		transport.NewMap(
			transport.NewPipe(transport.NewSpread[core.Primitive](), arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0)))),
		),
		transport.NewCollect[float64](),
	)
}
