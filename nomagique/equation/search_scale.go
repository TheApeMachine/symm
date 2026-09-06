package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewSearchScale composes sqrt(2*log(candidates)/observations), preserving the
// source lead/lag threshold formula. This expression alone does not establish
// a calibrated test for asynchronously overlapping, dependent returns.
func NewSearchScale(candidates, observations core.Primitive) core.Primitive {
	return transport.NewPipe(
		NewRatio[float64](
			NewProduct[float64](store.NewConstant(core.From(2.0)),
				transport.NewPipe(candidates, calculus.NewLog(transport.NewIO(core.From(0.0))))), observations),
		calculus.NewSqrt(transport.NewIO(core.From(0.0))),
	)
}
