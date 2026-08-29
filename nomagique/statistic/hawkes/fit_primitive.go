package hawkes

import (
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
ArrivalsSeries names the Frame-native bounded ring (nomagique/temporal.Path)
that retains marked arrivals: value = mark (+1 buy, -1 sell), timestamp =
event time. This is the ONLY place Hawkes market/model history lives — it is
one committed Frame slot set per symbol, owned by nomagique.Number, never a
package-level registry.

Composition order matters: ConditionalIntensity, Likelihood, Branching, and
Fit all read this series' CURRENT retained samples, which never include the
event being evaluated — ArrivalPath (temporal.Path(ArrivalsSeries prefix))
runs LAST in algo.Hawkes' Pipe, appending the current event only after every
primitive that judges it has already run. This ordering is what makes the
current event unable to excite or refit its own evaluation, per
signal/hawkes/README.md section 5.3.
*/
var ArrivalsSeries = temporal.NewSeries("hawkes/arrivals")

/*
ArrivalPath is the retention primitive: it relays the current event's mark
and event time onto the arrival series' own slots, then appends them to the
bounded ring. Compose it LAST in algo.Hawkes().
*/
var ArrivalPath = types.Pipe(
	types.Relay(SymbolMark, ArrivalsSeries.ValueSymbol),
	types.Relay(types.EventTimeSec, ArrivalsSeries.SecSymbol),
	types.Relay(types.EventTimeNsec, ArrivalsSeries.NsecSymbol),
	temporal.Path("hawkes/arrivals"),
)

/*
Fit-owned Frame facts. Model* is the fitted process in force BEFORE the
current event — the only state ConditionalIntensity, Branching, and
Likelihood may read to judge the current event. Fit reads the retained
arrival path (which, by the composition order above, excludes the current
event) and, only when the data-derived refit condition fires, overwrites
Model* with the newly estimated parameters for the NEXT event to read.
*/
var (
	SymbolModelMuX     = types.MustIntern("hawkes/state/model/mu_x")
	SymbolModelMuY     = types.MustIntern("hawkes/state/model/mu_y")
	SymbolModelAlphaXX = types.MustIntern("hawkes/state/model/alpha_xx")
	SymbolModelAlphaXY = types.MustIntern("hawkes/state/model/alpha_xy")
	SymbolModelAlphaYX = types.MustIntern("hawkes/state/model/alpha_yx")
	SymbolModelAlphaYY = types.MustIntern("hawkes/state/model/alpha_yy")
	SymbolModelBeta    = types.MustIntern("hawkes/state/model/beta")
	SymbolModelReady   = types.MustIntern("hawkes/state/model/ready")
	SymbolModelSupport = types.MustIntern("hawkes/state/model/support")
)

/*
Fit is the sole state-mutating boundary for the fitted Hawkes model. It
derives refit sufficiency and the retained-history horizon from the data
itself (arrivalTune.minFitEvents / tradeWindowDuration — never a fixed count
or duration), and — only on a converged, valid fit — overwrites Model* for
the next event to consume. A fit that does not converge, or is not yet
supported, leaves the previously committed Model* untouched: lack of a
usable fit is absent state, never a fallback constant.
*/
func Fit(input *types.Frame) {
	buy, sell, ok := retainedArrivals(input)

	if !ok {
		return
	}

	stream := newArrivalStream(buy, sell)
	horizonSec := eventHorizonSec(input)
	context, ok := newFitContext(stream, horizonSec)

	if !ok || !context.enoughEvents(stream) {
		return
	}

	support, _ := input.Get(SymbolModelSupport)
	ready, _ := input.Get(SymbolModelReady)

	if ready != 0 && context.totalEvents-int(support) < context.minFitEvents {
		return
	}

	fitted := newBivariateEstimator(priorFit(input)).fit(stream, horizonSec)

	if !fitted.valid() {
		return
	}

	input.Put(SymbolModelMuX, fitted.muX)
	input.Put(SymbolModelMuY, fitted.muY)
	input.Put(SymbolModelAlphaXX, fitted.alphaXX)
	input.Put(SymbolModelAlphaXY, fitted.alphaXY)
	input.Put(SymbolModelAlphaYX, fitted.alphaYX)
	input.Put(SymbolModelAlphaYY, fitted.alphaYY)
	input.Put(SymbolModelBeta, fitted.beta)
	input.Put(SymbolModelReady, 1)
	input.Put(SymbolModelSupport, float64(context.totalEvents))
}

func priorFit(input *types.Frame) bivariateFit {
	muX, muY, alphaXX, alphaXY, alphaYX, alphaYY, beta, ok := ReadModel(input)

	if !ok {
		return bivariateFit{}
	}

	fit := bivariateFit{
		muX: muX, muY: muY,
		alphaXX: alphaXX, alphaXY: alphaXY, alphaYX: alphaYX, alphaYY: alphaYY,
		beta: beta,
	}
	fit.spectralRadius = fit.computeSpectralRadius()

	return fit
}

/*
ReadModel returns the fitted model in force BEFORE the current event, or ok
false when no fit has ever converged. Frame-native primitives outside this
package use this instead of reaching into hawkes/state/model/* symbols
directly, keeping the fitted parameter representation an implementation
detail of this package.
*/
func ReadModel(input *types.Frame) (muX, muY, alphaXX, alphaXY, alphaYX, alphaYY, beta float64, ok bool) {
	ready, _ := input.Get(SymbolModelReady)

	if ready == 0 {
		return 0, 0, 0, 0, 0, 0, 0, false
	}

	muX, _ = input.Get(SymbolModelMuX)
	muY, _ = input.Get(SymbolModelMuY)
	alphaXX, _ = input.Get(SymbolModelAlphaXX)
	alphaXY, _ = input.Get(SymbolModelAlphaXY)
	alphaYX, _ = input.Get(SymbolModelAlphaYX)
	alphaYY, _ = input.Get(SymbolModelAlphaYY)
	beta, _ = input.Get(SymbolModelBeta)

	return muX, muY, alphaXX, alphaXY, alphaYX, alphaYY, beta, true
}

/*
retainedArrivals reads the arrival path retained BEFORE the current event
(ArrivalPath has not yet run in the composed Pipe) into two ascending
timestamp streams. ok is false when fewer than two events are retained,
since a Hawkes fit is not identifiable from less.
*/
func retainedArrivals(input *types.Frame) (buy, sell []float64, ok bool) {
	count := ArrivalsSeries.CountPtr(input)

	if count < 2 {
		return nil, nil, false
	}

	buy = make([]float64, 0, count)
	sell = make([]float64, 0, count)

	for index := 0; index < count; index++ {
		timestampNanos, mark, found := ArrivalsSeries.Sample(input, index)

		if !found {
			continue
		}

		atSec := float64(timestampNanos) * 1e-9

		if mark > 0 {
			buy = append(buy, atSec)

			continue
		}

		sell = append(sell, atSec)
	}

	return buy, sell, true
}

/*
eventHorizonSec resolves the current event's timestamp in the same seconds
epoch the retained arrival path uses.
*/
func eventHorizonSec(input *types.Frame) float64 {
	sec, _ := input.Get(types.EventTimeSec)
	nsec, _ := input.Get(types.EventTimeNsec)

	return sec + nsec*1e-9
}
