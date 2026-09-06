package vector

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewDot pairs two scalar streams, multiplies each pair and adds the products.
// Zip owns the shape check; no vector-specific arithmetic loop exists.
func NewDot(left, right core.Primitive) core.Primitive {
	return transport.NewPipe(
		transport.NewZip(left, right),
		transport.NewMapReduce(
			transport.NewPipe(transport.NewSpread[core.Primitive](), arithmetic.NewMultiply[float64](transport.NewIO(core.From(1.0)))),
			arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))),
		),
	)
}
