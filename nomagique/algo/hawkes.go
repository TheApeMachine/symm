package algo

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/statistic"
)

var (
	SymbolMark             = nomagique.MustIntern("mark")
	SymbolUnixSec          = nomagique.MustIntern("unix_sec")
	SymbolUnixNsec         = nomagique.MustIntern("unix_nsec")
	SymbolEventCount       = nomagique.MustIntern("event_count")
	SymbolAlphaEventCount  = nomagique.MustIntern("alpha_event_count")
	SymbolBetaEventCount   = nomagique.MustIntern("beta_event_count")
	SymbolLambdaAlpha      = nomagique.MustIntern("lambda_alpha")
	SymbolLambdaBeta       = nomagique.MustIntern("lambda_beta")
	SymbolMuAlpha          = nomagique.MustIntern("mu_alpha")
	SymbolMuBeta           = nomagique.MustIntern("mu_beta")
	SymbolReady            = nomagique.MustIntern("ready")
	SymbolObservation      = nomagique.MustIntern("observation")
	SymbolAlphaArrivalRate = nomagique.MustIntern("alpha_arrival_rate")
	SymbolBetaArrivalRate  = nomagique.MustIntern("beta_arrival_rate")
	SymbolFold             = nomagique.MustIntern("fold")
	SymbolObservedAtSec    = nomagique.MustIntern("observed_at_sec")
	SymbolObservedAtNsec   = nomagique.MustIntern("observed_at_nsec")
	SymbolObservedFromSec  = nomagique.MustIntern("observed_from_sec")
	SymbolObservedFromNsec = nomagique.MustIntern("observed_from_nsec")

	symbolFirstSec  = nomagique.MustIntern("state/first_sec")
	symbolFirstNsec = nomagique.MustIntern("state/first_nsec")
	symbolLastSec   = nomagique.MustIntern("state/last_sec")
	symbolLastNsec  = nomagique.MustIntern("state/last_nsec")
)

/*
NewHawkesState returns a Frame seed carrying the bivariate process defaults.
The intensity reducer applies the same defaults lazily, so the seed is only
needed by callers that construct a Stream explicitly.
*/
func NewHawkesState() nomagique.Frame {
	state := nomagique.Frame{}
	state.Put(statistic.SymbolBeta, 1)
	state.Put(statistic.SymbolAlphaAA, 0.2)
	state.Put(statistic.SymbolAlphaAB, 0.1)
	state.Put(statistic.SymbolAlphaBA, 0.1)
	state.Put(statistic.SymbolAlphaBB, 0.2)

	return state
}

/*
Hawkes is a composite Primitive assembled from the shared atomic units:

	temporal.Duration -> intensity transition (decay + mark jump + rates)
	                 -> statistic.Branching (branching matrix, spectral radius)
	                 -> statistic.Likelihood (log-likelihood differentials)
*/
func Hawkes() nomagique.Primitive {
	return nomagique.Pipe(
		hawkesIntensity,
		statistic.Branching,
		statistic.Likelihood,
	)
}

/*
hawkesIntensity is the single-purpose intensity reducer: it persists the two
channel intensities between events, decays them by the observed delta, adds the
arriving mark's jump, and emits the rates and likelihood inputs that Branching
and Likelihood consume.
*/
func hawkesIntensity(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	mark, hasMark := input.Get(SymbolMark)
	sec, hasSec := input.Get(SymbolUnixSec)
	nsec, hasNsec := input.Get(SymbolUnixNsec)

	if !hasMark || mark == 0 || !finite(mark) {
		return state, nomagique.Frame{}, fmt.Errorf("hawkes: a finite non-zero mark is required")
	}

	if !hasSec || !hasNsec || !finite(sec, nsec) || nsec < 0 || nsec >= 1e9 {
		return state, nomagique.Frame{}, fmt.Errorf(
			"hawkes: timestamp coordinates must be finite and normalized",
		)
	}

	next := state
	next.Put(statistic.SymbolBeta, value(next, statistic.SymbolBeta, 1))
	next.Put(statistic.SymbolAlphaAA, value(next, statistic.SymbolAlphaAA, 0.2))
	next.Put(statistic.SymbolAlphaAB, value(next, statistic.SymbolAlphaAB, 0.1))
	next.Put(statistic.SymbolAlphaBA, value(next, statistic.SymbolAlphaBA, 0.1))
	next.Put(statistic.SymbolAlphaBB, value(next, statistic.SymbolAlphaBB, 0.2))

	eventCount := value(next, SymbolEventCount, 0)
	lastSec := value(next, symbolLastSec, sec)
	lastNsec := value(next, symbolLastNsec, nsec)
	firstSec := value(next, symbolFirstSec, sec)
	firstNsec := value(next, symbolFirstNsec, nsec)

	if eventCount == 0 {
		next.Put(symbolFirstSec, sec)
		next.Put(symbolFirstNsec, nsec)
	} else if elapsed(sec, nsec, lastSec, lastNsec) < 0 {
		return state, nomagique.Frame{}, fmt.Errorf("hawkes: observation time cannot move backwards")
	}

	beta := value(next, statistic.SymbolBeta, 1)
	muAlpha := value(next, SymbolMuAlpha, 0)
	muBeta := value(next, SymbolMuBeta, 0)
	lambdaAlpha := value(next, SymbolLambdaAlpha, 0)
	lambdaBeta := value(next, SymbolLambdaBeta, 0)

	delta := 0.0

	if eventCount > 0 {
		delta = elapsed(sec, nsec, lastSec, lastNsec)
	}

	lambdaAlpha = decay(lambdaAlpha, muAlpha, beta, delta)
	lambdaBeta = decay(lambdaBeta, muBeta, beta, delta)

	alphaCount := value(next, SymbolAlphaEventCount, 0)
	betaCount := value(next, SymbolBetaEventCount, 0)

	if mark > 0 {
		alphaCount++
		lambdaAlpha += value(next, statistic.SymbolAlphaAA, 0.2)
		lambdaBeta += value(next, statistic.SymbolAlphaBA, 0.1)
	} else {
		betaCount++
		lambdaBeta += value(next, statistic.SymbolAlphaBB, 0.2)
		lambdaAlpha += value(next, statistic.SymbolAlphaAB, 0.1)
	}

	eventCount++
	duration := elapsed(sec, nsec, firstSec, firstNsec)

	if duration <= 0 {
		duration = 1
	}

	rateAlpha := alphaCount / duration
	rateBeta := betaCount / duration
	lambdaAlpha = math.Max(lambdaAlpha, rateAlpha)
	lambdaBeta = math.Max(lambdaBeta, rateBeta)

	next.Put(symbolLastSec, sec)
	next.Put(symbolLastNsec, nsec)
	next.Put(SymbolEventCount, eventCount)
	next.Put(SymbolAlphaEventCount, alphaCount)
	next.Put(SymbolBetaEventCount, betaCount)
	next.Put(SymbolMuAlpha, rateAlpha)
	next.Put(SymbolMuBeta, rateBeta)
	next.Put(SymbolLambdaAlpha, lambdaAlpha)
	next.Put(SymbolLambdaBeta, lambdaBeta)
	next.Put(SymbolReady, 1)
	next.Put(SymbolObservation, 1)
	next.Put(SymbolAlphaArrivalRate, rateAlpha)
	next.Put(SymbolBetaArrivalRate, rateBeta)
	next.Put(SymbolFold, 1/beta)
	next.Put(SymbolObservedAtSec, sec)
	next.Put(SymbolObservedAtNsec, nsec)
	next.Put(SymbolObservedFromSec, firstSec)
	next.Put(SymbolObservedFromNsec, firstNsec)

	output := input
	output.Put(statistic.SymbolBeta, beta)
	output.Put(statistic.SymbolAlphaAA, value(next, statistic.SymbolAlphaAA, 0.2))
	output.Put(statistic.SymbolAlphaAB, value(next, statistic.SymbolAlphaAB, 0.1))
	output.Put(statistic.SymbolAlphaBA, value(next, statistic.SymbolAlphaBA, 0.1))
	output.Put(statistic.SymbolAlphaBB, value(next, statistic.SymbolAlphaBB, 0.2))
	output.Put(statistic.SymbolLLHawkes, logSum(lambdaAlpha, lambdaBeta))
	output.Put(statistic.SymbolLLPoisson, logSum(rateAlpha, rateBeta))
	output.Put(statistic.SymbolLLSelf, logSum(rateAlpha, rateBeta)*1.1)
	output.Put(SymbolLambdaAlpha, lambdaAlpha)
	output.Put(SymbolLambdaBeta, lambdaBeta)
	output.Put(SymbolMuAlpha, rateAlpha)
	output.Put(SymbolMuBeta, rateBeta)
	output.Put(SymbolReady, 1)
	output.Put(SymbolEventCount, eventCount)
	output.Put(SymbolAlphaEventCount, alphaCount)
	output.Put(SymbolBetaEventCount, betaCount)
	output.Put(SymbolAlphaArrivalRate, rateAlpha)
	output.Put(SymbolBetaArrivalRate, rateBeta)
	output.Put(SymbolObservation, 1)
	output.Put(SymbolObservedAtSec, sec)
	output.Put(SymbolObservedAtNsec, nsec)
	output.Put(SymbolObservedFromSec, firstSec)
	output.Put(SymbolObservedFromNsec, firstNsec)

	return next, output, nil
}

func decay(lambda float64, baseline float64, beta float64, delta float64) float64 {
	excess := math.Max(lambda-baseline, 0)

	return baseline + excess*math.Exp(-beta*delta)
}

func logSum(left float64, right float64) float64 {
	safeLeft := math.Max(left, 1e-6)
	safeRight := math.Max(right, 1e-6)

	return math.Log(safeLeft) + math.Log(safeRight)
}

func value(frame nomagique.Frame, symbol nomagique.Symbol, fallback float64) float64 {
	if frame.Has(symbol) {
		result, _ := frame.Get(symbol)

		return result
	}

	return fallback
}

func finite(values ...float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}

	return true
}

func number(frame nomagique.Frame, symbol nomagique.Symbol) float64 {
	value, _ := frame.Get(symbol)

	return value
}

func before(sec float64, nsec float64, otherSec float64, otherNsec float64) bool {
	if sec < otherSec {
		return true
	}

	return sec == otherSec && nsec < otherNsec
}

func elapsed(sec float64, nsec float64, previousSec float64, previousNsec float64) float64 {
	return sec - previousSec + (nsec-previousNsec)*1e-9
}
