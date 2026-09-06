package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewSpacings takes consecutive timestamp differences in integer nanoseconds,
// converting only the resulting duration to floating point.
func NewSpacings() core.Primitive {
	return transport.NewPipe(
		transport.NewWindow(2, 1),
		transport.NewMap(
			transport.NewPipe(
				NewDifference[int64](
					transport.NewPipe(collection.NewAt[core.Primitive](transport.NewIO(core.From(1.0))), store.NewGet("at")),
					transport.NewPipe(collection.NewAt[core.Primitive](transport.NewIO(core.From(0.0))), store.NewGet("at")),
				),
				calculus.NewConvert[int64, float64](),
			),
		),
	)
}
