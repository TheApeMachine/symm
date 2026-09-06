package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewMomentRetention is the original sample-mass shedding equation. It emits
// replacements for count and m2 while keeping the mean unchanged. Selection of
// when to shed belongs to a separate gate/controller composition.
func NewMomentRetention() core.Primitive {
	return transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				NewProduct[float64](store.NewGet("count"), store.NewGet("retain")),
				calculus.NewMaximum(transport.NewIO(core.From(2.0))),
				store.NewKey("new_count"),
			),
		),
		store.NewRecord(
			transport.NewPipe(store.NewGet("new_count"), store.NewKey("count")),
			transport.NewPipe(store.NewGet("mean"), store.NewKey("mean")),
			transport.NewPipe(
				NewProduct[float64](
					store.NewGet("m2"),
					NewRatio[float64](
						NewDifference[float64](store.NewGet("new_count"), store.NewConstant(core.From(1.0))),
						NewDifference[float64](store.NewGet("count"), store.NewConstant(core.From(1.0))),
					),
				),
				store.NewKey("m2"),
			),
		),
	)
}
