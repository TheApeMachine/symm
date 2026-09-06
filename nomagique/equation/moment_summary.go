package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewMomentSummary adds Bessel-corrected sample variance and dispersion. A
// single observation has undefined sample variance, not an invented zero.
func NewMomentSummary() core.Primitive {
	return transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				NewRatio[float64](
					store.NewGet("m2"), NewDifference[float64](store.NewGet("count"), store.NewConstant(core.From(1.0)))),
				store.NewKey("variance"),
			),
		),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(store.NewGet("variance"),
				calculus.NewSqrt(transport.NewIO(core.From(0.0))), store.NewKey("dispersion")),
		),
	)
}
