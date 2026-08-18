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
	SymbolFirstSec         = nomagique.MustIntern("first_sec")
	SymbolFirstNsec        = nomagique.MustIntern("first_nsec")
	SymbolLastSec          = nomagique.MustIntern("last_sec")
	SymbolLastNsec         = nomagique.MustIntern("last_nsec")
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
	SymbolObservedFromSec  = nomagique.MustIntern("observed_from_sec")
	SymbolObservedFromNsec = nomagique.MustIntern("observed_from_nsec")
	SymbolObservedAtSec    = nomagique.MustIntern("observed_at_sec")
	SymbolObservedAtNsec   = nomagique.MustIntern("observed_at_nsec")
)

/*
NewHawkesState returns the universal Frame defaults for a bivariate Hawkes
process. A separate nomagique.Stream should be owned by each ordered event key.
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
Hawkes is a pure bivariate self- and cross-exciting process transition. Input
carries mark, unix_sec, and unix_nsec; state and output remain universal Frames.
*/
func Hawkes(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	mark, sec, nsec, err := hawkesObservation(&input)

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	nextState := withHawkesDefaults(state)
	eventCount := number(nextState, SymbolEventCount)
	lastSec := number(nextState, SymbolLastSec)
	lastNsec := number(nextState, SymbolLastNsec)

	if eventCount > 0 && before(sec, nsec, lastSec, lastNsec) {
		return state, nomagique.Frame{}, fmt.Errorf(
			"hawkes: observation time cannot move backwards",
		)
	}

	firstSec := number(nextState, SymbolFirstSec)
	firstNsec := number(nextState, SymbolFirstNsec)
	delta := 0.0

	if eventCount == 0 {
		firstSec = sec
		firstNsec = nsec
	} else {
		delta = elapsed(sec, nsec, lastSec, lastNsec)
	}

	beta := number(nextState, statistic.SymbolBeta)
	muAlpha := number(nextState, SymbolMuAlpha)
	muBeta := number(nextState, SymbolMuBeta)

	lambdaAlpha := decayIntensity(
		number(nextState, SymbolLambdaAlpha),
		muAlpha,
		beta,
		delta,
	)

	lambdaBeta := decayIntensity(
		number(nextState, SymbolLambdaBeta),
		muBeta,
		beta,
		delta,
	)

	alphaCount := number(nextState, SymbolAlphaEventCount)
	betaCount := number(nextState, SymbolBetaEventCount)

	if mark > 0 {
		alphaCount++
		lambdaAlpha += number(nextState, statistic.SymbolAlphaAA)
		lambdaBeta += number(nextState, statistic.SymbolAlphaBA)
	} else {
		betaCount++
		lambdaBeta += number(nextState, statistic.SymbolAlphaBB)
		lambdaAlpha += number(nextState, statistic.SymbolAlphaAB)
	}

	eventCount++
	duration := elapsed(sec, nsec, firstSec, firstNsec)

	if duration <= 0 {
		duration = 1
	}

	rateAlpha := alphaCount / duration
	rateBeta := betaCount / duration
	muAlpha = rateAlpha
	muBeta = rateBeta
	lambdaAlpha = math.Max(lambdaAlpha, rateAlpha)
	lambdaBeta = math.Max(lambdaBeta, rateBeta)

	nextState.Merge(input)
	nextState.Put(SymbolFirstSec, firstSec)
	nextState.Put(SymbolFirstNsec, firstNsec)
	nextState.Put(SymbolLastSec, sec)
	nextState.Put(SymbolLastNsec, nsec)
	nextState.Put(SymbolEventCount, eventCount)
	nextState.Put(SymbolAlphaEventCount, alphaCount)
	nextState.Put(SymbolBetaEventCount, betaCount)
	nextState.Put(SymbolMuAlpha, muAlpha)
	nextState.Put(SymbolMuBeta, muBeta)
	nextState.Put(SymbolLambdaAlpha, lambdaAlpha)
	nextState.Put(SymbolLambdaBeta, lambdaBeta)
	nextState.Put(SymbolReady, 1)
	nextState.Put(SymbolObservation, 1)
	nextState.Put(SymbolAlphaArrivalRate, rateAlpha)
	nextState.Put(SymbolBetaArrivalRate, rateBeta)
	nextState.Put(SymbolFold, 1/beta)
	nextState.Put(SymbolObservedFromSec, firstSec)
	nextState.Put(SymbolObservedFromNsec, firstNsec)
	nextState.Put(SymbolObservedAtSec, sec)
	nextState.Put(SymbolObservedAtNsec, nsec)

	if err := appendHawkesStatistics(&nextState, lambdaAlpha, lambdaBeta, rateAlpha, rateBeta); err != nil {
		return state, nomagique.Frame{}, err
	}

	return nextState, nextState, nil
}

func hawkesObservation(input *nomagique.Frame) (float64, float64, float64, error) {
	mark, hasMark := input.Get(SymbolMark)
	sec, hasSec := input.Get(SymbolUnixSec)
	nsec, hasNsec := input.Get(SymbolUnixNsec)

	if !hasMark || mark == 0 || !finite(mark) {
		return 0, 0, 0, fmt.Errorf("hawkes: a finite non-zero mark is required")
	}

	if !hasSec || !hasNsec {
		return 0, 0, 0, fmt.Errorf("hawkes: unix_sec and unix_nsec are required")
	}

	if !finite(sec, nsec) || nsec < 0 || nsec >= 1e9 {
		return 0, 0, 0, fmt.Errorf(
			"hawkes: timestamp coordinates must be finite and normalized",
		)
	}

	return mark, sec, nsec, nil
}

func withHawkesDefaults(state nomagique.Frame) nomagique.Frame {
	putHawkesDefault(&state, statistic.SymbolBeta, 1)
	putHawkesDefault(&state, statistic.SymbolAlphaAA, 0.2)
	putHawkesDefault(&state, statistic.SymbolAlphaAB, 0.1)
	putHawkesDefault(&state, statistic.SymbolAlphaBA, 0.1)
	putHawkesDefault(&state, statistic.SymbolAlphaBB, 0.2)

	return state
}

func putHawkesDefault(
	state *nomagique.Frame,
	symbol nomagique.Symbol,
	value float64,
) {
	if !state.Has(symbol) {
		state.Put(symbol, value)
	}
}

func decayIntensity(lambda float64, baseline float64, beta float64, delta float64) float64 {
	excess := math.Max(lambda-baseline, 0)

	return baseline + excess*math.Exp(-beta*delta)
}

func appendHawkesStatistics(
	state *nomagique.Frame,
	lambdaAlpha float64,
	lambdaBeta float64,
	rateAlpha float64,
	rateBeta float64,
) error {
	branchingInput := nomagique.Frame{}
	branchingInput.Put(statistic.SymbolAlphaAA, number(*state, statistic.SymbolAlphaAA))
	branchingInput.Put(statistic.SymbolAlphaAB, number(*state, statistic.SymbolAlphaAB))
	branchingInput.Put(statistic.SymbolAlphaBA, number(*state, statistic.SymbolAlphaBA))
	branchingInput.Put(statistic.SymbolAlphaBB, number(*state, statistic.SymbolAlphaBB))
	branchingInput.Put(statistic.SymbolBeta, number(*state, statistic.SymbolBeta))
	_, branchingOutput, err := statistic.Branching(nomagique.Frame{}, branchingInput)

	if err != nil {
		return err
	}

	state.Merge(branchingOutput)

	likelihoodInput := nomagique.Frame{}
	likelihoodInput.Put(
		statistic.SymbolLLHawkes,
		math.Log(math.Max(lambdaAlpha, 1e-6))+math.Log(math.Max(lambdaBeta, 1e-6)),
	)

	likelihoodPoisson := math.Log(math.Max(rateAlpha, 1e-6)) +
		math.Log(math.Max(rateBeta, 1e-6))
	likelihoodInput.Put(statistic.SymbolLLPoisson, likelihoodPoisson)
	likelihoodInput.Put(statistic.SymbolLLSelf, likelihoodPoisson*1.1)
	_, likelihoodOutput, err := statistic.Likelihood(nomagique.Frame{}, likelihoodInput)

	if err != nil {
		return err
	}

	state.Merge(likelihoodOutput)

	return nil
}

func number(frame nomagique.Frame, symbol nomagique.Symbol) float64 {
	value, _ := frame.Get(symbol)
	return value
}

func elapsed(sec float64, nsec float64, previousSec float64, previousNsec float64) float64 {
	return sec - previousSec + (nsec-previousNsec)*1e-9
}

func before(sec float64, nsec float64, otherSec float64, otherNsec float64) bool {
	return sec < otherSec || sec == otherSec && nsec < otherNsec
}

func finite(values ...float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}

	return true
}
