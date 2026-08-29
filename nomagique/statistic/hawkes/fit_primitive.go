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

Composition order matters: ConditionalIntensity, Likelihood, Branching,
Excitation, and Compensator all read this series' samples BEFORE ArrivalPath
appends the current event (they run earlier in algo.Hawkes' Pipe), so the
current event cannot excite or influence its own evaluation. ArrivalPath then
appends it, and Fit — which runs LAST — reads the series AFTER that append:
per signal/hawkes/README.md section 5.3's causal ordering ("incorporate the
event into process state" precedes "refit/update model parameters for
subsequent observations"), a refit is entitled to use the event it just
incorporated, but the model it produces only takes effect starting with the
NEXT event, since every primitive that judges an event already ran earlier
in this same Pipe call.
*/
var ArrivalsSeries = temporal.NewSeries("hawkes/arrivals")

/*
ArrivalPath is the retention primitive: it relays the current event's mark
and event time onto the arrival series' own slots, then appends them to the
bounded ring. Compose it after every primitive that judges the current event
and before Fit in algo.Hawkes().
*/
var ArrivalPath = types.Pipe(
	types.Relay(SymbolMark, ArrivalsSeries.ValueSymbol),
	types.Relay(types.EventTimeSec, ArrivalsSeries.SecSymbol),
	types.Relay(types.EventTimeNsec, ArrivalsSeries.NsecSymbol),
	temporal.Path("hawkes/arrivals"),
)

/*
Fit-owned Frame facts. Model* is the fitted process in force BEFORE the
current event — the only state ConditionalIntensity, Branching, Excitation,
Likelihood, and Compensator may read to judge the current event. Fit runs
after ArrivalPath and reads the arrival path INCLUDING the current event
(the just-incorporated observation the README requires the refit to be
entitled to use) and, only when the data-derived refit condition fires,
overwrites Model* with the newly estimated parameters for the NEXT event to
read.
*/
var (
	SymbolModelMuX            = types.MustIntern("hawkes/state/model/mu_x")
	SymbolModelMuY            = types.MustIntern("hawkes/state/model/mu_y")
	SymbolModelAlphaXX        = types.MustIntern("hawkes/state/model/alpha_xx")
	SymbolModelAlphaXY        = types.MustIntern("hawkes/state/model/alpha_xy")
	SymbolModelAlphaYX        = types.MustIntern("hawkes/state/model/alpha_yx")
	SymbolModelAlphaYY        = types.MustIntern("hawkes/state/model/alpha_yy")
	SymbolModelBeta           = types.MustIntern("hawkes/state/model/beta")
	SymbolModelReady          = types.MustIntern("hawkes/state/model/ready")
	SymbolModelSupport        = types.MustIntern("hawkes/state/model/support")
	symbolModelEventsSinceFit = types.MustIntern("hawkes/state/model/events_since_fit")

	// Self-only baseline model, refitted at the same cadence as the full
	// model: README sections 18/34 require the self-only comparison to be an
	// independently fitted restricted model on the same support, not the
	// full model with cross terms zeroed after the fact (that would report
	// how much a WORSE self-only fit the full model implies, not how well
	// the best possible self-only model actually explains the data).
	symbolSelfOnlyMuX     = types.MustIntern("hawkes/state/self_only/mu_x")
	symbolSelfOnlyMuY     = types.MustIntern("hawkes/state/self_only/mu_y")
	symbolSelfOnlyAlphaXX = types.MustIntern("hawkes/state/self_only/alpha_xx")
	symbolSelfOnlyAlphaYY = types.MustIntern("hawkes/state/self_only/alpha_yy")
	symbolSelfOnlyBeta    = types.MustIntern("hawkes/state/self_only/beta")
	symbolSelfOnlyReady   = types.MustIntern("hawkes/state/self_only/ready")
)

/*
Fit is the sole state-mutating boundary for the fitted Hawkes model. It
derives refit sufficiency from the data itself (arrivalTune.minFitEvents —
never a fixed count), and — only on a converged, valid fit — overwrites
Model* for the next event to consume. A fit that does not converge, or is
not yet supported, leaves the previously committed Model* untouched: lack of
a usable fit is absent state, never a fallback constant.

Refit cadence counts events, not ring-retained support: the arrival path's
retention is capacity-bounded (nomagique/temporal.MaxPathSamples), so once
enough events have passed for the ring to be full and rolling,
context.totalEvents itself stops growing. Gating a refit on
"context.totalEvents has grown by minFitEvents since the last fit" would
therefore latch permanently false the moment the ring first fills — the
model would fossilize at whatever it fitted during warmup and never refit
again for the rest of the process's life. symbolModelEventsSinceFit instead
counts every Fit call since the last successful refit, uncapped by ring
capacity, and is the quantity compared against minFitEvents.

Deferred while the current event shares its timestamp with the immediately
preceding retained event: per signal/hawkes/README.md section 4.2,
simultaneous events must be treated as a batch, so no event in a same-instant
cluster may see a model that a refit — triggered by an earlier event in that
same cluster — updated mid-batch. Refitting is deferred to the first
distinct-timestamp event after the cluster closes, at which point the whole
batch is already retained and the fit's own likelihood correctly evaluates
it together (intensityAt/excitationSum already exclude zero-age
contributions within a batch; this defers PARAMETER updates the same way).
*/
func Fit(input *types.Frame) {
	buy, sell, ok := retainedArrivals(input)

	if !ok {
		return
	}

	if sharesTimestampWithPredecessor(input) {
		return
	}

	stream := newArrivalStream(buy, sell)
	horizonSec := eventHorizonSec(input)
	context, ok := newFitContext(stream, horizonSec)

	if !ok || !context.enoughEvents(stream) {
		return
	}

	eventsSinceFit, _ := input.Get(symbolModelEventsSinceFit)
	eventsSinceFit++
	ready, _ := input.Get(SymbolModelReady)

	if ready != 0 && int(eventsSinceFit) < context.minFitEvents {
		input.Put(symbolModelEventsSinceFit, eventsSinceFit)

		return
	}

	estimator := newBivariateEstimator(priorFit(input))
	fitted := estimator.fit(stream, horizonSec)

	if !fitted.valid() {
		input.Put(symbolModelEventsSinceFit, eventsSinceFit)

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
	input.Put(symbolModelEventsSinceFit, 0)

	selfOnly := estimator.fitSelfOnly(stream, horizonSec)

	if !selfOnly.valid() {
		return
	}

	input.Put(symbolSelfOnlyMuX, selfOnly.muX)
	input.Put(symbolSelfOnlyMuY, selfOnly.muY)
	input.Put(symbolSelfOnlyAlphaXX, selfOnly.alphaXX)
	input.Put(symbolSelfOnlyAlphaYY, selfOnly.alphaYY)
	input.Put(symbolSelfOnlyBeta, selfOnly.beta)
	input.Put(symbolSelfOnlyReady, 1)
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
ReadSelfOnlyModel returns the independently fitted self-only baseline (cross
excitation constrained to zero, per README section 18) in force BEFORE the
current event, or ok false when it has never converged.
*/
func ReadSelfOnlyModel(input *types.Frame) (muX, muY, alphaXX, alphaYY, beta float64, ok bool) {
	ready, _ := input.Get(symbolSelfOnlyReady)

	if ready == 0 {
		return 0, 0, 0, 0, 0, false
	}

	muX, _ = input.Get(symbolSelfOnlyMuX)
	muY, _ = input.Get(symbolSelfOnlyMuY)
	alphaXX, _ = input.Get(symbolSelfOnlyAlphaXX)
	alphaYY, _ = input.Get(symbolSelfOnlyAlphaYY)
	beta, _ = input.Get(symbolSelfOnlyBeta)

	return muX, muY, alphaXX, alphaYY, beta, true
}

/*
retainedArrivals reads the arrival path currently retained on the Frame into
two ascending timestamp streams. Every primitive up through Compensator
calls this BEFORE ArrivalPath has run (so the current event is excluded);
Fit calls it AFTER ArrivalPath has run (so the current event is included),
per algo.Hawkes' composition order. ok is false only when the series has
never been written at all (a brand new symbol) — an empty or single-event
retained history is a legitimate, valid observation state that callers such
as Accounting must still report on. Fit-specific identifiability minimums
belong to Fit's own context.enoughEvents gate, not to reading history.
*/
func retainedArrivals(input *types.Frame) (buy, sell []float64, ok bool) {
	count := ArrivalsSeries.CountPtr(input)

	if count == 0 {
		return nil, nil, true
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
sharesTimestampWithPredecessor reports whether the most recently retained
event (the current event, since Fit calls this after ArrivalPath) shares its
exact timestamp with the event immediately before it. Called only from Fit.
*/
func sharesTimestampWithPredecessor(input *types.Frame) bool {
	count := ArrivalsSeries.CountPtr(input)

	if count < 2 {
		return false
	}

	currentNanos, _, foundCurrent := ArrivalsSeries.Sample(input, count-1)
	previousNanos, _, foundPrevious := ArrivalsSeries.Sample(input, count-2)

	return foundCurrent && foundPrevious && currentNanos == previousNanos
}

/*
eventHorizonSec resolves the current event's timestamp through the exact
same integral-nanoseconds-then-convert path retainedArrivals uses for
retained samples. Computing "the same instant" two different ways (this
event's own sec+nsec*1e-9 versus a retained sample's int64-nanoseconds*1e-9)
does not round-trip to a bit-identical float64 at Unix-epoch magnitude: the
two paths can disagree by roughly 1e-7 seconds, enough for
excitationSum's strict eventTime > horizonSec test to spuriously exclude an
event that is actually AT the horizon, depending on which side of that
razor's edge the rounding happens to land on for a given timestamp. Routing
both derivations through eventNanos keeps that comparison exact.
*/
func eventHorizonSec(input *types.Frame) float64 {
	return float64(eventNanos(input)) * 1e-9
}

/*
eventNanos resolves the current event's timestamp as integral Unix
nanoseconds, matching temporal.Path's own internal encoding exactly.
*/
func eventNanos(input *types.Frame) int64 {
	sec, _ := input.Get(types.EventTimeSec)
	nsec, _ := input.Get(types.EventTimeNsec)

	return int64(sec)*1_000_000_000 + int64(nsec)
}
