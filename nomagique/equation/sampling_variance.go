package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewSamplingVariance applies specificity debt to this reading's own context.
// Context coordinates are float64 counts; impossible matched depths fail.
func NewSamplingVariance() core.Primitive {
	return logic.NewGate(
		NewLessEqual[float64](store.NewGet("depth"), store.NewGet("context_length")),
		NewRatio[float64](
			store.NewGet("variance"),
			transport.NewPipe(
				NewRatio[float64](
					store.NewGet("support"),
					NewSum[float64](store.NewConstant(core.From(1.0)),
						NewDifference[float64](store.NewGet("context_length"), store.NewGet("depth"))),
				),
				calculus.NewMaximum(transport.NewIO(core.From(1.0))),
			),
		),
		logic.NewReject(core.ErrDomain),
	)
}
