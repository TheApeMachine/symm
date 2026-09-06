package equation

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewElapsed subtracts int64 nanoseconds before conversion to seconds. In
// particular, epoch magnitude cannot erase a small interval by cancellation.
func NewElapsed(from, through core.Primitive) core.Primitive {
	return transport.NewPipe(
		NewDifference[int64](through, from),
		calculus.NewConvert[int64, float64](),
		arithmetic.NewMultiply[float64](transport.NewIO(core.From(1e-9))),
	)
}
