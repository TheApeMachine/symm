package data

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewReadout composes credibility, corroboration and the usable raw value.
// supports/contradictions yield authority values already resolved by their own
// configured graphs. discrete supplies the caller's coordinate/ordinal policy.
// No receiver introspection, child type assertion or recursive raw-value helper
// is needed; a child may be another configured Readout composition.
func NewReadout(authority, supports, contradictions, credibility, discrete core.Primitive) core.Primitive {
	prepare := store.NewRecord(
		transport.NewPipe(),
		transport.NewPipe(authority, store.NewKey("base_authority")),
		transport.NewPipe(supports, arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))), store.NewKey("support_weight")),
		transport.NewPipe(
			contradictions,
			arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))),
			store.NewKey("contradiction_weight"),
		),
		transport.NewPipe(
			equation.NewBound(credibility, store.NewConstant(core.From(0.0)), store.NewConstant(core.From(1.0))),
			store.NewKey("credibility"),
		),
		transport.NewPipe(discrete, store.NewKey("discrete")),
		transport.NewPipe(
			logic.NewGate(store.NewHas("defined"), store.NewGet("defined"), store.NewConstant(core.From(true))),
			store.NewKey("defined"),
		),
	)
	resolved := logic.NewGate(
		store.NewGet("defined"),
		equation.NewBound(
			equation.NewProduct[float64](
				equation.NewProduct[float64](store.NewGet("base_authority"), store.NewGet("credibility")),
				equation.NewRatio[float64](
					equation.NewSum[float64](store.NewConstant(core.From(1.0)), store.NewGet("support_weight")),
					equation.NewSum[float64](
						store.NewConstant(core.From(1.0)),
						equation.NewProduct[float64](store.NewConstant(core.From(2.0)), store.NewGet("contradiction_weight")),
					),
				),
			),
			store.NewConstant(core.From(0.0)),
			store.NewConstant(core.From(1.0)),
		),
		store.NewConstant(core.From(0.0)),
	)
	return transport.NewPipe(
		prepare,
		store.NewRecord(transport.NewPipe(), transport.NewPipe(resolved, store.NewKey("authority"))),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				logic.NewGate(
					store.NewGet("discrete"),
					store.NewGet("raw"),
					equation.NewProduct[float64](store.NewGet("raw"), store.NewGet("authority")),
				),
				store.NewKey("value"),
			),
		),
	)
}
