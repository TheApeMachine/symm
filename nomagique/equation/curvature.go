package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewCurvature composes neighbouring profile readings. x coordinates determine
// units; LagProfile supplies seconds, so curvature uses inverse squared seconds.
// Edge peaks have no neighbours and report ErrShape rather than old evidence.
func NewCurvature() core.Primitive {
	center := store.NewRetained(core.From(0.0))
	return transport.NewPipe(
		transport.NewCollect[core.Primitive](),
		transport.NewFan(
			transport.NewPipe(),
			transport.NewIO(
				transport.NewPipe(transport.NewSpread[core.Primitive](), NewPeak(), store.NewGet("index"), center, transport.NewDiscard()),
				transport.NewPipe(),
			),
		),
		NewRatio[float64](
			NewProduct[float64](
				NewDifference[float64](
					transport.NewPipe(
						collection.NewAt[core.Primitive](transport.NewApply(transport.NewPipe(), center)),
						store.NewGet("y"),
						calculus.NewAbsolute(transport.NewIO(core.From(0.0))),
					),
					NewProduct[float64](
						NewSum[float64](
							transport.NewPipe(
								collection.NewAt[core.Primitive](
									transport.NewApply(NewSum[float64](transport.NewPipe(), store.NewConstant(core.From(-1.0))), center),
								),
								store.NewGet("y"),
								calculus.NewAbsolute(transport.NewIO(core.From(0.0))),
							),
							transport.NewPipe(
								collection.NewAt[core.Primitive](
									transport.NewApply(NewSum[float64](transport.NewPipe(), store.NewConstant(core.From(1.0))), center),
								),
								store.NewGet("y"),
								calculus.NewAbsolute(transport.NewIO(core.From(0.0))),
							),
						),
						store.NewConstant(core.From(0.5)),
					),
				),
				store.NewConstant(core.From(2.0)),
			),
			transport.NewPipe(
				NewProduct[float64](
					NewDifference[float64](
						transport.NewPipe(
							collection.NewAt[core.Primitive](
								transport.NewApply(NewSum[float64](transport.NewPipe(), store.NewConstant(core.From(1.0))), center),
							),
							store.NewGet("x"),
						),
						transport.NewPipe(
							collection.NewAt[core.Primitive](
								transport.NewApply(NewSum[float64](transport.NewPipe(), store.NewConstant(core.From(-1.0))), center),
							),
							store.NewGet("x"),
						),
					),
					store.NewConstant(core.From(0.5)),
				),
				calculus.NewSquare(transport.NewIO(core.From(0.0))),
			),
		),
	)
}
