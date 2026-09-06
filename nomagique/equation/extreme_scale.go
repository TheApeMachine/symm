package equation

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewExtremeScale evaluates sqrt(2*log(count)), the coefficient used by the
// supplied EVT and Gaussian envelope modes. It adds no tail calibration claim.
func NewExtremeScale() core.Primitive {
	return transport.NewPipe(
		store.NewGet("count"),
		calculus.NewLog(transport.NewIO(core.From(0.0))),
		arithmetic.NewMultiply[float64](transport.NewIO(core.From(2.0))),
		calculus.NewSqrt(transport.NewIO(core.From(0.0))),
	)
}
