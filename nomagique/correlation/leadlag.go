package correlation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewLeadLag composes the supplied search policy around an opaque estimator.
// Resolution is the finer path's median spacing; span is min(path counts)-2.
// The profile output preserves every estimator record and its own support.
// x is seconds, spacing is nanoseconds. The winning nonzero lag is compared
// with contemporaneous dependence. Empty searches are explicit undefined records.
// The inherited search_scale is a policy quantity, not a calibrated p-value.
func NewLeadLag(estimator core.Primitive) core.Primitive {
	context := store.NewRetained(nil)
	profile := store.NewRetained(nil)
	peak := store.NewRetained(nil)
	spacing := transport.NewApply(store.NewGet("spacing"), context)
	span := transport.NewApply(store.NewGet("span"), context)
	profileValues := transport.NewApply(transport.NewSpread[core.Primitive](), profile)
	nonzero := transport.NewPipe(profileValues,
		transport.NewMap(logic.NewGate(
			equation.NewAll(store.NewGet("defined"),
				transport.NewPipe(equation.NewEqual[float64](store.NewGet("x"), store.NewConstant(core.From(0.0))),
					logic.NewNot(transport.NewIO(core.From(false))))),
			transport.NewPipe(), transport.NewDiscard())))
	undefined := store.NewRecord(transport.NewPipe(),
		transport.NewPipe(store.NewConstant(core.From([]core.Primitive{})), store.NewKey("profile")),
		transport.NewPipe(store.NewConstant(core.From(false)), store.NewKey("defined")),
		transport.NewPipe(store.NewConstant(core.From(false)), store.NewKey("leads")),
		transport.NewPipe(store.NewConstant(core.From(false)), store.NewKey("shape_defined")))

	summarize := transport.NewPipe(
		nonzero, equation.NewPeak(), store.NewGet("point"), peak,
		store.NewRecord(transport.NewPipe(),
			transport.NewPipe(profileValues, transport.NewCollect[core.Primitive](), store.NewKey("profile")),
			transport.NewPipe(transport.NewApply(store.NewGet("zero"), context), store.NewGet("correlation"), store.NewKey("contemporaneous")),
			transport.NewPipe(transport.NewApply(store.NewGet("search_count"), context), store.NewKey("search_count")),
			transport.NewPipe(spacing, store.NewKey("spacing")),
			transport.NewPipe(span, store.NewKey("span")),
			transport.NewPipe(transport.NewApply(store.NewGet("observations"), context), store.NewKey("observations"))),
		store.NewRecord(transport.NewPipe(),
			transport.NewPipe(equation.NewSearchScale(
				equation.NewSum[float64](store.NewGet("search_count"), store.NewConstant(core.From(1.0))),
				equation.NewDifference[float64](store.NewGet("observations"), store.NewConstant(core.From(1.0)))), store.NewKey("search_scale")),
			transport.NewPipe(equation.NewDifference[float64](
				transport.NewPipe(store.NewGet("correlation"), calculus.NewAbsolute(transport.NewIO(core.From(0.0)))),
				transport.NewPipe(store.NewGet("contemporaneous"), calculus.NewAbsolute(transport.NewIO(core.From(0.0))))), store.NewKey("absolute_gain"))),
		store.NewRecord(transport.NewPipe(),
			transport.NewPipe(equation.NewAll(
				equation.NewGreater[float64](transport.NewPipe(store.NewGet("correlation"), calculus.NewAbsolute(transport.NewIO(core.From(0.0)))), store.NewGet("search_scale")),
				equation.NewGreater[float64](store.NewGet("absolute_gain"), store.NewConstant(core.From(0.0)))), store.NewKey("leads")),
			transport.NewPipe(equation.NewRatio[float64](store.NewGet("x"),
				equation.NewProduct[float64](store.NewGet("spacing"), store.NewConstant(core.From(1e-9)))), store.NewKey("lag_index"))),
		store.NewRecord(transport.NewPipe(),
			transport.NewPipe(logic.NewGate(store.NewGet("leads"),
				equation.NewRatio[float64](transport.NewPipe(store.NewGet("lag_index"), calculus.NewAbsolute(transport.NewIO(core.From(0.0)))), store.NewGet("span")),
				store.NewConstant(core.From(0.0))), store.NewKey("lag_fraction")),
			transport.NewPipe(equation.NewSum[float64](store.NewGet("lag_index"), store.NewGet("span")), store.NewKey("index"))),
		peak,
		NewLagShape(profile, peak),
	)
	search := transport.NewPipe(
		store.NewRecord(transport.NewPipe(),
			transport.NewPipe(equation.NewMinimum(
				transport.NewPipe(store.NewGet("left"), transport.NewSpread[core.Primitive](), equation.NewSpacings(), equation.NewMedian()),
				transport.NewPipe(store.NewGet("right"), transport.NewSpread[core.Primitive](), equation.NewSpacings(), equation.NewMedian())), store.NewKey("spacing")),
			transport.NewPipe(equation.NewDifference[float64](store.NewGet("observations"), store.NewConstant(core.From(2.0))), store.NewKey("span"))),
		context,
		logic.NewGate(equation.NewGreater[float64](store.NewGet("spacing"), store.NewConstant(core.From(0.0))),
			transport.NewPipe(
				equation.NewLagProfile(NewDependence(estimator), spacing, span),
				transport.NewCollect[core.Primitive](), profile,
				store.NewRecord(
					transport.NewPipe(transport.NewApply(context, nil)),
					transport.NewPipe(collection.NewAt[core.Primitive](span), store.NewKey("zero")),
					transport.NewPipe(nonzero, equation.NewCount(), store.NewKey("search_count"))),
				context,
				logic.NewGate(equation.NewGreater[float64](store.NewGet("search_count"), store.NewConstant(core.From(0.0))),
					summarize, undefined)),
			undefined),
	)
	return logic.NewGate(equation.NewAll(store.NewHas("left"), store.NewHas("right")),
		transport.NewPipe(
			store.NewRecord(transport.NewPipe(),
				transport.NewPipe(equation.NewMinimum(
					transport.NewPipe(store.NewGet("left"), transport.NewSpread[core.Primitive](), equation.NewCount()),
					transport.NewPipe(store.NewGet("right"), transport.NewSpread[core.Primitive](), equation.NewCount())), store.NewKey("observations"))),
			logic.NewGate(equation.NewGreater[float64](store.NewGet("observations"), store.NewConstant(core.From(2.0))), search, undefined)),
		logic.NewReject(core.ErrShape))
}
