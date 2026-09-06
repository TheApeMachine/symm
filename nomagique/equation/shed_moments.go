package equation

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewShedMoments applies the source's support-shedding domain, without changing
// its mean or sample dispersion. Non-contraction requests leave the record
// unchanged. The policy chooses when; this composition defines what shedding is.
func NewShedMoments() core.Primitive {
	return logic.NewGate(
		NewAll(
			NewGreater[float64](store.NewGet("retain"), store.NewConstant(core.From(0.0))),
			NewLess[float64](store.NewGet("retain"), store.NewConstant(core.From(1.0))),
			NewGreater[float64](store.NewGet("count"), store.NewConstant(core.From(2.0))),
		),
		store.NewRecord(transport.NewPipe(), NewMomentRetention()),
		transport.NewPipe(),
	)
}
