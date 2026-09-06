package equation

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewMedian orders a captured run and averages its two central order statistics.
// NaN is explicit undefinedness; infinities remain ordered values. An empty run
// has no central index and is reported by At, rather than returning stale data.
func NewMedian() core.Primitive {
	ordered := store.NewRetained(core.From([]float64{}))
	lower := transport.NewApply(
		transport.NewPipe(
			transport.NewSpread[float64](),
			NewCount(),
			NewDifference[float64](transport.NewPipe(), store.NewConstant(core.From(1.0))),
			arithmetic.NewMultiply[float64](transport.NewIO(core.From(0.5))),
			calculus.NewFloor(transport.NewIO(core.From(0.0))),
		),
		ordered,
	)
	upper := transport.NewApply(
		transport.NewPipe(
			transport.NewSpread[float64](),
			NewCount(),
			arithmetic.NewMultiply[float64](transport.NewIO(core.From(0.5))),
			calculus.NewFloor(transport.NewIO(core.From(0.0))),
		),
		ordered,
	)
	return logic.NewGate(
		transport.NewMapReduce(logic.NewIsNaN(), logic.NewOr(transport.NewIO(core.From(false)))),
		calculus.NewMinimum(transport.NewIO(core.From(0.0))),
		transport.NewPipe(
			transport.NewCollect[float64](),
			collection.NewOrder[float64](),
			ordered,
			transport.NewFan(transport.NewPipe(), transport.NewIO(collection.NewAt[float64](lower), collection.NewAt[float64](upper))),
			arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))),
			arithmetic.NewMultiply[float64](transport.NewIO(core.From(0.5))),
		),
	)
}
