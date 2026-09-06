package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
)

// NewSoftmax captures the maximum, shifts each logit, exponentiates, normalizes.
// Non-finite logits fail explicitly rather than producing plausible certainty.
func NewSoftmax() core.Primitive {
	maximum := store.NewRetained(core.From(-math.MaxFloat64))
	return transport.NewPipe(
		transport.NewMap(logic.NewGate(logic.NewFinite(), transport.NewPipe(), logic.NewReject(core.ErrShape))),
		logic.NewGate(
			NewGreater[float64](NewCount(), store.NewConstant(core.From(0.0))),
			transport.NewPipe(),
			logic.NewReject(core.ErrNotHeld),
		),
		transport.NewFan(
			transport.NewPipe(),
			transport.NewIO(
				transport.NewPipe(calculus.NewMaximum(transport.NewIO(core.From(-math.MaxFloat64))), maximum, transport.NewDiscard()),
				transport.NewPipe(),
			),
		),
		transport.NewMap(
			transport.NewPipe(
				NewDifference[float64](transport.NewPipe(), transport.NewApply(maximum, nil)),
				calculus.NewExp(transport.NewIO(core.From(0.0))),
			),
		),
		NewNormalize(),
	)
}
