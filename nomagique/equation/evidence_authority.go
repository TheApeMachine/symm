package equation

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
)

// NewEvidenceAuthority is the supplied measurement's maturity/SNR weighting.
// Unknown-SNR and zero-SNR factors are configuration, not new statistical laws.
func NewEvidenceAuthority(unknown, zero core.Primitive) core.Primitive {
	factor := logic.NewGate(
		store.NewGet("estimated"),
		logic.NewGate(
			store.NewGet("snr_defined"),
			logic.NewGate(
				NewGreater[float64](store.NewGet("snr"), store.NewConstant(core.From(0.0))),
				NewRatio[float64](store.NewGet("snr"), NewSum[float64](store.NewConstant(core.From(1.0)), store.NewGet("snr"))),
				zero,
			),
			unknown,
		),
		store.NewConstant(core.From(1.0)),
	)
	return NewBound(
		NewProduct[float64](
			NewBound(store.NewGet("maturity"), store.NewConstant(core.From(0.0)), store.NewConstant(core.From(1.0))),
			factor,
		),
		store.NewConstant(core.From(0.0)),
		store.NewConstant(core.From(1.0)),
	)
}
