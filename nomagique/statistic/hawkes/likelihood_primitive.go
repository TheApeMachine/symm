package hawkes

import "github.com/theapemachine/symm/nomagique/types"

/*
Likelihood-scoped Frame facts, per signal/hawkes/README.md sections 16-18.
These are recomputed fresh from the currently retained window every call,
never accumulated across refits: ℓ_H(θ) is defined for one fitted θ over one
observation interval [From, At], and a running sum of per-event terms
computed under DIFFERENT θ (one per intervening refit) would not equal
ℓ_H(θ) for any single θ.
*/
var (
	SymbolLLHawkesTotal    = types.MustIntern("hawkes/obs/log_likelihood_hawkes")
	SymbolLLPoissonTotal   = types.MustIntern("hawkes/obs/log_likelihood_poisson")
	SymbolLLSelfTotal      = types.MustIntern("hawkes/obs/log_likelihood_self_only")
	SymbolLLHawkesPerEvent = types.MustIntern("hawkes/obs/log_likelihood_per_event_hawkes")
	SymbolLLGainPoisson    = types.MustIntern("hawkes/obs/log_likelihood_gain_vs_poisson")
	SymbolLLGainSelf       = types.MustIntern("hawkes/obs/log_likelihood_gain_vs_self_only")
	SymbolLLGainPoissonPer = types.MustIntern("hawkes/obs/log_likelihood_gain_per_event_vs_poisson")
	SymbolLLGainSelfPer    = types.MustIntern("hawkes/obs/log_likelihood_gain_per_event_vs_self_only")
)

/*
Likelihood evaluates the exact bivariate Hawkes log-likelihood over [From,
At] — the retained window PLUS the current event's own pre-arrival
contribution, per README section 5.3 step 2 ("evaluate likelihood... "
happens before "incorporate the event into process state") — under the
model fitted before this event, together with the marked-Poisson baseline
(alpha=0, the same fitted mu: the Poisson mu that maximizes its own
likelihood on this support IS the fitted rate, so no separate optimization
is needed for it) and the independently, restrictedly fitted self-only
baseline (ReadSelfOnlyModel — README section 18 requires an actual
restricted refit on the same support, not the full model with cross terms
zeroed after the fact). Recomputed fresh every call from the full currently
retained window, never accumulated, so a mid-window refit cannot leave the
reported total representing a mix of models. Requires a converged fit;
absent one, every symbol here stays absent.
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

	mark, hasMark := input.Get(SymbolMark)

	if !hasMark || mark == 0 {
		return
	}

	horizonSec := eventHorizonSec(input)
	stream := currentWindowStream(buy, sell, horizonSec, mark)

	fit := bivariateFit{
		muX: muX, muY: muY,
		alphaXX: alphaXX, alphaXY: alphaXY, alphaYX: alphaYX, alphaYY: alphaYY,
		beta: beta,
	}
	poisson := bivariateFit{muX: muX, muY: muY, beta: beta}

	hawkesLL := fit.logLikelihood(stream, horizonSec)
	poissonLL := poisson.logLikelihood(stream, horizonSec)

	if !finite(hawkesLL, poissonLL) {
		return
	}

	eventCount := float64(len(stream.marked))

	if eventCount <= 0 {
		return
	}

	input.Put(SymbolLLHawkesTotal, hawkesLL)
	input.Put(SymbolLLPoissonTotal, poissonLL)
	input.Put(SymbolLLHawkesPerEvent, hawkesLL/eventCount)
	input.Put(SymbolLLGainPoisson, hawkesLL-poissonLL)
	input.Put(SymbolLLGainPoissonPer, (hawkesLL-poissonLL)/eventCount)

	selfMuX, selfMuY, selfAlphaXX, selfAlphaYY, selfBeta, selfOK := ReadSelfOnlyModel(input)

	if !selfOK {
		return
	}

	selfOnly := bivariateFit{
		muX: selfMuX, muY: selfMuY,
		alphaXX: selfAlphaXX, alphaYY: selfAlphaYY,
		beta: selfBeta,
	}
	selfLL := selfOnly.logLikelihood(stream, horizonSec)

	if !finite(selfLL) {
		return
	}

	input.Put(SymbolLLSelfTotal, selfLL)
	input.Put(SymbolLLGainSelf, hawkesLL-selfLL)
	input.Put(SymbolLLGainSelfPer, (hawkesLL-selfLL)/eventCount)
}

/*
currentWindowStream builds the observation window [From, At] this
measurement reports over: the retained buy/sell history PLUS the current
event's own mark at horizonSec, so its own pre-arrival intensity contributes
a log-likelihood term the way README section 5.3 requires, without ever
retaining that synthetic entry back into the Frame — ArrivalPath, not this
function, owns committing the current event to history.
*/
func currentWindowStream(buy, sell []float64, horizonSec, mark float64) arrivalStream {
	if mark > 0 {
		buy = append(append([]float64(nil), buy...), horizonSec)
	} else {
		sell = append(append([]float64(nil), sell...), horizonSec)
	}

	return newArrivalStream(buy, sell)
}
