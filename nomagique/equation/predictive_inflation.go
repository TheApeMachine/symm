package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewPredictiveInflation evaluates sqrt(1+1/count) on a moment record.
func NewPredictiveInflation() core.Primitive {
	return transport.NewPipe(
		NewSum[float64](store.NewConstant(core.From(1.0)),
			NewRatio[float64](store.NewConstant(core.From(1.0)), store.NewGet("count"))),
		calculus.NewSqrt(transport.NewIO(core.From(0.0))),
	)
}
