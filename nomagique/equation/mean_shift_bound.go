package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewMeanShiftBound preserves the supplied window's bound. It is not a claim
// that its approximate retained-moment window implements the full ADWIN paper.
// Inputs: variance, observations, recent_count, prior_count.
func NewMeanShiftBound() core.Primitive {
	return transport.NewPipe(
		NewProduct[float64](
			store.NewGet("variance"),
			NewProduct[float64](
				transport.NewPipe(
					NewProduct[float64](
						store.NewConstant(core.From(4.0)),
						transport.NewPipe(store.NewGet("observations"), calculus.NewSquare(transport.NewIO(core.From(0.0)))),
					),
					calculus.NewLog(transport.NewIO(core.From(0.0))),
				),
				NewProduct[float64](
					store.NewConstant(core.From(0.5)),
					NewSum[float64](
						NewRatio[float64](store.NewConstant(core.From(1.0)), store.NewGet("recent_count")),
						NewRatio[float64](store.NewConstant(core.From(1.0)), store.NewGet("prior_count")),
					),
				),
			),
		),
		calculus.NewSqrt(transport.NewIO(core.From(0.0))),
	)
}
