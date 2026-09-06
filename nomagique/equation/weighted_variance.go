package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewWeightedVariance is E_w[x^2]-E_w[x]^2 over {weight,value} records. It is
// population dispersion, not an unbiased sample-variance correction. The
// nonnegative floor preserves the source cohort's roundoff convention; NaN
// remains undefined. There is no second accumulation implementation.
func NewWeightedVariance() core.Primitive {
	return transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(NewWeightedMean(), store.NewKey("mean")),
			transport.NewPipe(
				transport.NewMap(store.NewRecord(
					transport.NewPipe(store.NewGet("weight"), store.NewKey("weight")),
					transport.NewPipe(store.NewGet("value"), calculus.NewSquare(transport.NewIO(core.From(0.0))), store.NewKey("value")))),
				NewWeightedMean(), store.NewKey("second_moment"))),
		NewDifference[float64](store.NewGet("second_moment"),
			transport.NewPipe(store.NewGet("mean"), calculus.NewSquare(transport.NewIO(core.From(0.0))))),
		calculus.NewMaximum(transport.NewIO(core.From(0.0))),
	)
}
