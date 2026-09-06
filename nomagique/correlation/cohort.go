package correlation

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewCohort summarizes one peer run of {correlation,support,peer_energy?} records.
// Support below two and nonfinite correlations are excluded, with counts exposed.
// The caller supplies the dispersion transform (normally Atanh). It is applied
// once per admitted peer. No clamp is hidden in this composition: an undefined
// transform makes fisher_defined false rather than fabricating finite evidence.
// A new delivery run is a new cohort; no Reset method or replay convention exists.
func NewCohort(transform core.Primitive) core.Primitive {
	admitted := transport.NewPipe(
		transport.NewMap(logic.NewGate(
			equation.NewAll(store.NewHas("correlation"), store.NewHas("support")),
			logic.NewGate(equation.NewAll(
				transport.NewPipe(store.NewGet("correlation"), logic.NewFinite()),
				transport.NewPipe(store.NewGet("support"), logic.NewFinite()),
				equation.NewLessEqual[float64](store.NewConstant(core.From(2.0)), store.NewGet("support"))),
				store.NewRecord(transport.NewPipe(),
					transport.NewPipe(logic.NewGate(store.NewHas("peer_energy"), store.NewGet("peer_energy"),
						store.NewConstant(core.From(0.0))), store.NewKey("peer_energy")),
					transport.NewPipe(store.NewGet("correlation"), transform, store.NewKey("transformed"))),
				transport.NewDiscard()),
			logic.NewReject(core.ErrShape))),
		transport.NewCollect[core.Primitive](),
		store.NewKey("admitted"),
	)
	values := transport.NewPipe(store.NewGet("admitted"), transport.NewSpread[core.Primitive]())
	weights := transport.NewPipe(values, transport.NewMap(store.NewGet("support")))
	return transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(equation.NewCount(), store.NewKey("peers_seen")), admitted),
		store.NewRecord(
			transport.NewPipe(store.NewGet("peers_seen"), store.NewKey("peers_seen")),
			transport.NewPipe(values, equation.NewCount(), store.NewKey("peers")),
			transport.NewPipe(weights, arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))), store.NewKey("total_support")),
			transport.NewPipe(weights, equation.NewKish(), store.NewKey("effective_peers")),
			transport.NewPipe(values, transport.NewMap(store.NewRecord(
				transport.NewPipe(store.NewGet("support"), store.NewKey("weight")),
				transport.NewPipe(store.NewGet("correlation"), store.NewKey("value")))),
				equation.NewWeightedMean(), store.NewKey("signed_correlation")),
			transport.NewPipe(values, transport.NewMap(store.NewRecord(
				transport.NewPipe(store.NewGet("support"), store.NewKey("weight")),
				transport.NewPipe(store.NewGet("correlation"), calculus.NewAbsolute(transport.NewIO(core.From(0.0))), store.NewKey("value")))),
				equation.NewWeightedMean(), store.NewKey("absolute_correlation")),
			transport.NewPipe(values, transport.NewMap(store.NewRecord(
				transport.NewPipe(store.NewGet("support"), store.NewKey("weight")),
				transport.NewPipe(store.NewGet("peer_energy"), store.NewKey("value")))),
				equation.NewWeightedMean(), store.NewKey("peer_energy_rate")),
			transport.NewPipe(values, transport.NewMap(store.NewRecord(
				transport.NewPipe(store.NewGet("support"), store.NewKey("weight")),
				transport.NewPipe(store.NewGet("transformed"), store.NewKey("value")))),
				equation.NewWeightedVariance(), calculus.NewSqrt(transport.NewIO(core.From(0.0))), store.NewKey("dispersion"))),
		store.NewRecord(transport.NewPipe(),
			transport.NewPipe(equation.NewGreater[float64](store.NewGet("total_support"), store.NewConstant(core.From(0.0))), store.NewKey("defined")),
			transport.NewPipe(transport.NewPipe(store.NewGet("dispersion"), logic.NewFinite()), store.NewKey("fisher_defined")),
			transport.NewPipe(equation.NewDifference[float64](store.NewGet("peers_seen"), store.NewGet("peers")), store.NewKey("rejected_peers"))),
	)
}
