package statistic

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolMark             = types.MustIntern("mark")
	SymbolEventCount       = types.MustIntern("event_count")
	SymbolAlphaEventCount  = types.MustIntern("alpha_event_count")
	SymbolBetaEventCount   = types.MustIntern("beta_event_count")
	SymbolLambdaAlpha      = types.MustIntern("lambda_alpha")
	SymbolLambdaBeta       = types.MustIntern("lambda_beta")
	SymbolMuAlpha          = types.MustIntern("mu_alpha")
	SymbolMuBeta           = types.MustIntern("mu_beta")
	SymbolObservation      = types.MustIntern("observation")
	SymbolAlphaArrivalRate = types.MustIntern("alpha_arrival_rate")
	SymbolBetaArrivalRate  = types.MustIntern("beta_arrival_rate")
	SymbolFold             = types.MustIntern("fold")
	SymbolObservedAtSec    = types.MustIntern("observed_at_sec")
	SymbolObservedAtNsec   = types.MustIntern("observed_at_nsec")
	SymbolObservedFromSec  = types.MustIntern("observed_from_sec")
	SymbolObservedFromNsec = types.MustIntern("observed_from_nsec")

	symbolFirstSec  = types.MustIntern("state/first_sec")
	symbolFirstNsec = types.MustIntern("state/first_nsec")
	symbolLastSec   = types.MustIntern("state/last_sec")
	symbolLastNsec  = types.MustIntern("state/last_nsec")

	symbolLambdaSelfAlpha = types.MustIntern("state/lambda_self_alpha")
	symbolLambdaSelfBeta  = types.MustIntern("state/lambda_self_beta")
)

/*
NewHawkesState returns a Frame seed carrying the bivariate process defaults.
The intensity reducer applies the same defaults lazily, so the seed is only
needed by callers that construct a Stream explicitly.
*/
func NewHawkesState() types.Frame {
	state := types.Frame{}
	state.Put(SymbolBeta, 1)
	state.Put(SymbolAlphaAA, 0.2)
	state.Put(SymbolAlphaAB, 0.1)
	state.Put(SymbolAlphaBA, 0.1)
	state.Put(SymbolAlphaBB, 0.2)

	return state
}

/*
HawkesIntensity is the single-purpose intensity reducer: it persists the two
channel intensities between events, decays them by the observed delta, adds the
arriving mark's jump, and emits the rates and likelihood inputs that Branching
and Likelihood consume.
*/
func HawkesIntensity(input types.Frame) types.Frame {
	mark, hasMark := input.Get(SymbolMark)
	sec, hasSec := input.Get(SymbolUnixSec)
	nsec, hasNsec := input.Get(SymbolUnixNsec)

	if !hasMark || mark == 0 || !finite(mark) {
		input.Err = fmt.Errorf("hawkes: a finite non-zero mark is required")

		return input
	}

	if !hasSec || !hasNsec || !finite(sec, nsec) || nsec < 0 || nsec >= 1e9 {
		input.Err = fmt.Errorf(
			"hawkes: timestamp coordinates must be finite and normalized",
		)

		return input
	}

	input.Put(SymbolBeta, value(input, SymbolBeta, 1))
	input.Put(SymbolAlphaAA, value(input, SymbolAlphaAA, 0.2))
	input.Put(SymbolAlphaAB, value(input, SymbolAlphaAB, 0.1))
	input.Put(SymbolAlphaBA, value(input, SymbolAlphaBA, 0.1))
	input.Put(SymbolAlphaBB, value(input, SymbolAlphaBB, 0.2))

	eventCount := value(input, SymbolEventCount, 0)
	lastSec := value(input, symbolLastSec, sec)
	lastNsec := value(input, symbolLastNsec, nsec)
	firstSec := value(input, symbolFirstSec, sec)
	firstNsec := value(input, symbolFirstNsec, nsec)

	if eventCount == 0 {
		input.Put(symbolFirstSec, sec)
		input.Put(symbolFirstNsec, nsec)
	} else if elapsed(sec, nsec, lastSec, lastNsec) < 0 {
		input.Err = fmt.Errorf("hawkes: observation time cannot move backwards")

		return input
	}

	beta := value(input, SymbolBeta, 1)

	if beta <= 0 || !finite(beta) {
		input.Err = fmt.Errorf("hawkes: beta must be positive and finite")

		return input
	}

	muAlpha := value(input, SymbolMuAlpha, 0)
	muBeta := value(input, SymbolMuBeta, 0)
	lambdaAlpha := value(input, SymbolLambdaAlpha, 0)
	lambdaBeta := value(input, SymbolLambdaBeta, 0)

	delta := 0.0

	if eventCount > 0 {
		delta = elapsed(sec, nsec, lastSec, lastNsec)
	}

	lambdaAlpha = decay(lambdaAlpha, muAlpha, beta, delta)
	lambdaBeta = decay(lambdaBeta, muBeta, beta, delta)

	alphaCount := value(input, SymbolAlphaEventCount, 0)
	betaCount := value(input, SymbolBetaEventCount, 0)

	if mark > 0 {
		alphaCount++
		lambdaAlpha += value(input, SymbolAlphaAA, 0.2)
		lambdaBeta += value(input, SymbolAlphaBA, 0.1)
	}

	if mark < 0 {
		betaCount++
		lambdaBeta += value(input, SymbolAlphaBB, 0.2)
		lambdaAlpha += value(input, SymbolAlphaAB, 0.1)
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

	input.Put(symbolLastSec, sec)
	input.Put(symbolLastNsec, nsec)
	input.Put(SymbolEventCount, eventCount)
	input.Put(SymbolAlphaEventCount, alphaCount)
	input.Put(SymbolBetaEventCount, betaCount)
	input.Put(SymbolMuAlpha, rateAlpha)
	input.Put(SymbolMuBeta, rateBeta)
	input.Put(SymbolLambdaAlpha, lambdaAlpha)
	input.Put(SymbolLambdaBeta, lambdaBeta)
	input.Put(SymbolReady, 1)
	input.Put(SymbolObservation, 1)
	input.Put(SymbolAlphaArrivalRate, rateAlpha)
	input.Put(SymbolBetaArrivalRate, rateBeta)
	input.Put(SymbolFold, 1/beta)
	input.Put(SymbolObservedAtSec, sec)
	input.Put(SymbolObservedAtNsec, nsec)
	input.Put(SymbolObservedFromSec, firstSec)
	input.Put(SymbolObservedFromNsec, firstNsec)

	lambdaSelfAlpha := decay(value(input, symbolLambdaSelfAlpha, rateAlpha), rateAlpha, beta, delta)
	lambdaSelfBeta := decay(value(input, symbolLambdaSelfBeta, rateBeta), rateBeta, beta, delta)

	if mark > 0 {
		lambdaSelfAlpha += value(input, SymbolAlphaAA, 0.2)
	}

	if mark < 0 {
		lambdaSelfBeta += value(input, SymbolAlphaBB, 0.2)
	}

	input.Put(symbolLambdaSelfAlpha, lambdaSelfAlpha)
	input.Put(symbolLambdaSelfBeta, lambdaSelfBeta)
	input.Put(SymbolLLHawkes, logSum(lambdaAlpha, lambdaBeta))
	input.Put(SymbolLLPoisson, logSum(rateAlpha, rateBeta))
	input.Put(SymbolLLSelf, logSum(lambdaSelfAlpha, lambdaSelfBeta))

	return input
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

func value(frame types.Frame, symbol types.Symbol, fallback float64) float64 {
	if frame.Has(symbol) {
		result, _ := frame.Get(symbol)

		return result
	}

	return fallback
}

func elapsed(sec float64, nsec float64, previousSec float64, previousNsec float64) float64 {
	return sec - previousSec + (nsec-previousNsec)*1e-9
}
