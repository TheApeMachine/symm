package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
)

// NewFisher composes the Fisher-z normal-tail formula for records containing
// "correlation" and "support". This formula does not establish calibration for
// dependent asynchronous overlaps. Invalid domains propagate NaN/Inf normally.
func NewFisher() core.Primitive {
	return transport.NewPipe(
		NewProduct[float64](
			transport.NewPipe(store.NewGet("correlation"), calculus.NewAtanh(transport.NewIO(core.From(0.0)))),
			transport.NewPipe(
				NewDifference[float64](store.NewGet("support"), store.NewConstant(core.From(3.0))),
				calculus.NewSqrt(transport.NewIO(core.From(0.0))),
			),
		),
		calculus.NewAbsolute(transport.NewIO(core.From(0.0))),
		NewRatio[float64](transport.NewPipe(), store.NewConstant(core.From(math.Sqrt2))),
		calculus.NewErfc(transport.NewIO(core.From(0.0))),
	)
}
