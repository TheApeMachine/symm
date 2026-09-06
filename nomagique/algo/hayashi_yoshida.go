package algo

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewHayashiYoshida is the named asynchronous covariance/correlation composer.
// The input is KV with left/right observation collections. Each observation is
// KV with at (int64 nanoseconds) and value (float64). Output keys are covariance,
// left_energy, right_energy, support, and correlation.
//
// Returns contribute once to their own energy and once to every overlapping
// cross-product. Support is overlap cardinality, not an independent sample size.
func NewHayashiYoshida() core.Primitive {
	return transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(
				store.NewGet("left"),
				transport.NewSpread[core.Primitive](),
				equation.NewLogReturns(),
				transport.NewCollect[core.Primitive](),
				store.NewKey("left"),
			),
			transport.NewPipe(
				store.NewGet("right"),
				transport.NewSpread[core.Primitive](),
				equation.NewLogReturns(),
				transport.NewCollect[core.Primitive](),
				store.NewKey("right"),
			),
		),
		store.NewRecord(
			transport.NewPipe(
				store.NewGet("left"),
				transport.NewSpread[core.Primitive](),
				transport.NewMap(store.NewGet("value")),
				equation.NewEnergy(),
				store.NewKey("left_energy"),
			),
			transport.NewPipe(
				store.NewGet("right"),
				transport.NewSpread[core.Primitive](),
				transport.NewMap(store.NewGet("value")),
				equation.NewEnergy(),
				store.NewKey("right_energy"),
			),
			transport.NewPipe(
				transport.NewCross(
					transport.NewPipe(store.NewGet("left"), transport.NewSpread[core.Primitive]()),
					transport.NewPipe(store.NewGet("right"), transport.NewSpread[core.Primitive]()),
				),
				transport.NewMap(logic.NewGate(equation.NewIntervalOverlap(), transport.NewPipe(), transport.NewDiscard())),
				store.NewRecord(
					transport.NewPipe(
						transport.NewMapReduce(
							equation.NewProduct[float64](
								transport.NewPipe(collection.NewAt[core.Primitive](transport.NewIO(core.From(0.0))), store.NewGet("value")),
								transport.NewPipe(collection.NewAt[core.Primitive](transport.NewIO(core.From(1.0))), store.NewGet("value")),
							),
							arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))),
						),
						store.NewKey("covariance"),
					),
					transport.NewPipe(equation.NewCount(), store.NewKey("support")),
				),
			),
		),
		store.NewRecord(transport.NewPipe(), transport.NewPipe(equation.NewCorrelation(), store.NewKey("correlation"))),
	)
}
