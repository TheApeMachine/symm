package hawkes

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Likelihood-scoped Frame facts. Totals accumulate across every event since the
first retained observation; per-event forms divide by the retained event
count, per signal/hawkes/README.md sections 16-18.
*/
var (
	SymbolLLHawkesTotal    = types.MustIntern("hawkes/state/ll_hawkes_total")
	SymbolLLPoissonTotal   = types.MustIntern("hawkes/state/ll_poisson_total")
	SymbolLLSelfTotal      = types.MustIntern("hawkes/state/ll_self_total")
	SymbolLLHawkesPerEvent = types.MustIntern("hawkes/obs/log_likelihood_per_event_hawkes")
	SymbolLLGainPoisson    = types.MustIntern("hawkes/obs/log_likelihood_gain_vs_poisson")
	SymbolLLGainSelf       = types.MustIntern("hawkes/obs/log_likelihood_gain_vs_self_only")
	SymbolLLGainPoissonPer = types.MustIntern("hawkes/obs/log_likelihood_gain_per_event_vs_poisson")
	SymbolLLGainSelfPer    = types.MustIntern("hawkes/obs/log_likelihood_gain_per_event_vs_self_only")
)

/*
Likelihood evaluates the exact bivariate Hawkes log-likelihood contribution
of the current event under the model fitted before it — sum of log
conditional intensities at the event marks (from ConditionalIntensity's
pre-arrival values) minus the compensator over the interval since the last
event — together with the same quantity for the marked-Poisson baseline
(alpha=0, same fitted mu) and the self-only baseline (cross-excitation
zeroed), per README sections 16-18. It requires a converged fit; absent one,
every symbol here stays absent rather than reporting a degenerate value.
*/
func Likelihood(input *types.Frame) {
	muX, muY, alphaXX, alphaXY, alphaYX, alphaYY, beta, ok := ReadModel(input)

	if !ok {
		return
	}

	buy, sell, ok := retainedArrivals(input)

	if !ok {
		buy, sell = nil, nil
	}

	stream := newArrivalStream(buy, sell)
	horizonSec := eventHorizonSec(input)
	mark, hasMark := input.Get(SymbolMark)

	if !hasMark || mark == 0 {
		input.Err = fmt.Errorf("hawkes: a nonzero mark is required")

		return
	}

	fit := bivariateFit{
		muX: muX, muY: muY,
		alphaXX: alphaXX, alphaXY: alphaXY, alphaYX: alphaYX, alphaYY: alphaYY,
		beta: beta,
	}
	poisson := bivariateFit{muX: muX, muY: muY, beta: beta}
	selfOnly := bivariateFit{
		muX: muX, muY: muY,
		alphaXX: alphaXX, alphaYY: alphaYY,
		beta: beta,
	}

	eventLikelihood := eventLogLikelihoodContribution(fit, stream, horizonSec, mark)
	poissonLikelihood := eventLogLikelihoodContribution(poisson, stream, horizonSec, mark)
	selfLikelihood := eventLogLikelihoodContribution(selfOnly, stream, horizonSec, mark)

	if !finite(eventLikelihood, poissonLikelihood, selfLikelihood) {
		return
	}

	hawkesTotal, _ := input.Get(SymbolLLHawkesTotal)
	poissonTotal, _ := input.Get(SymbolLLPoissonTotal)
	selfTotal, _ := input.Get(SymbolLLSelfTotal)
	hawkesTotal += eventLikelihood
	poissonTotal += poissonLikelihood
	selfTotal += selfLikelihood

	eventCount := float64(len(buy) + len(sell) + 1)

	input.Put(SymbolLLHawkesTotal, hawkesTotal)
	input.Put(SymbolLLPoissonTotal, poissonTotal)
	input.Put(SymbolLLSelfTotal, selfTotal)
	input.Put(SymbolLLHawkesPerEvent, hawkesTotal/eventCount)
	input.Put(SymbolLLGainPoisson, hawkesTotal-poissonTotal)
	input.Put(SymbolLLGainSelf, hawkesTotal-selfTotal)
	input.Put(SymbolLLGainPoissonPer, (hawkesTotal-poissonTotal)/eventCount)
	input.Put(SymbolLLGainSelfPer, (hawkesTotal-selfTotal)/eventCount)
}

/*
eventLogLikelihoodContribution isolates the current event's own contribution
to the exact log-likelihood: log intensity of the event's own mark at its own
pre-arrival value, minus the compensator increment over the interval opened
by the immediately preceding retained event (or the whole span, for the
first event). This is exactly the sum-of-logs-minus-integral form in README
section 16, evaluated one event at a time so it composes with the Accumulate
pattern the rest of this pipeline already uses for running totals.
*/
func eventLogLikelihoodContribution(
	fit bivariateFit,
	priorStream arrivalStream,
	horizonSec float64,
	mark float64,
) float64 {
	if fit.muX <= 0 || fit.muY <= 0 || fit.beta <= 0 {
		return math.Inf(-1)
	}

	lambdaBuy := intensityAt(priorStream.buy, priorStream.sell, horizonSec, fit.muX, fit.alphaXX, fit.alphaXY, fit.beta)
	lambdaSell := intensityAt(priorStream.buy, priorStream.sell, horizonSec, fit.muY, fit.alphaYX, fit.alphaYY, fit.beta)

	lambdaEvent := lambdaBuy

	if mark < 0 {
		lambdaEvent = lambdaSell
	}

	if lambdaEvent <= 0 {
		return math.Inf(-1)
	}

	originSec := priorStream.originSec
	lastEventSec := originSec

	if len(priorStream.marked) > 0 {
		lastEventSec = priorStream.marked[len(priorStream.marked)-1].atSec
	}

	span := horizonSec - lastEventSec

	if span <= 0 {
		return math.Log(lambdaEvent)
	}

	buySupport := observationKernelIntegralSupport(priorStream.buy, lastEventSec, horizonSec, fit.beta)
	sellSupport := observationKernelIntegralSupport(priorStream.sell, lastEventSec, horizonSec, fit.beta)

	buyIntegral := fit.muX*span + (fit.alphaXX/fit.beta)*buySupport + (fit.alphaXY/fit.beta)*sellSupport
	sellIntegral := fit.muY*span + (fit.alphaYX/fit.beta)*buySupport + (fit.alphaYY/fit.beta)*sellSupport

	return math.Log(lambdaEvent) - buyIntegral - sellIntegral
}
