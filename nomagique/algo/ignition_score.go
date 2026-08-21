package algo

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
)

const (
	sideAlpha = true
	sideBeta  = false
)

var (
	SymbolIgnitionRateReady    = nomagique.MustIntern("ignition/rate_ready")
	SymbolIgnitionMoveBaseline = nomagique.MustIntern("ignition/move_baseline")
	SymbolIgnitionMoveReady    = nomagique.MustIntern("ignition/move_ready")

	SymbolAlphaRejection = nomagique.MustIntern("alpha/rejection")
	SymbolAlphaFade      = nomagique.MustIntern("alpha/fade")
	SymbolAlphaDecline   = nomagique.MustIntern("alpha/decline")
	SymbolBetaRejection  = nomagique.MustIntern("beta/rejection")
	SymbolBetaFade       = nomagique.MustIntern("beta/fade")
	SymbolBetaDecline    = nomagique.MustIntern("beta/decline")
)

/*
ignitionBaselines exposes only causal tape history: both centers existed before
the completed bar currently being classified.
*/
func ignitionBaselines(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	rateBaseline, rateReady, err := ignitionHistoryMedian(&state, historyRates)

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	moveBaseline, moveReady, err := ignitionHistoryMedian(&state, historyReturns)

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	output := input
	output.Put(SymbolIgnitionRateBaseline, rateBaseline)
	output.Put(SymbolIgnitionRateReady, boolNumber(rateReady))
	output.Put(SymbolIgnitionMoveBaseline, moveBaseline)
	output.Put(SymbolIgnitionMoveReady, boolNumber(moveReady))

	return state, output, nil
}

/*
ignitionRelativeVolume is the rate/baseline Ratio followed by the universal
positive Squash. Its only constant is the dimensionless neutral ratio one.
*/
func ignitionRelativeVolume() nomagique.Primitive {
	return nomagique.Pipe(
		ignitionRateRatioOperands,
		calculus.Ratio,
		nomagique.Relay(calculus.SymbolResult, SymbolRVOL),
		ignitionRVOLScale,
		calculus.Squash,
		nomagique.Relay(calculus.SymbolResult, SymbolRVOLNormalized),
		ignitionRVOLLift,
	)
}

func ignitionRateRatioOperands(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	barRate, hasRate := input.Get(calculus.SymbolRate)
	baseline, hasBaseline := input.Get(SymbolIgnitionRateBaseline)
	ready, hasReady := input.Get(SymbolIgnitionRateReady)

	if !hasRate || !hasBaseline || !hasReady {
		return state, nomagique.Frame{}, fmt.Errorf(
			"ignition: rate equation and causal baseline are required",
		)
	}

	output := input
	output.Put(calculus.SymbolValue, barRate)
	output.Put(calculus.SymbolBaseline, baseline)
	output.Put(calculus.SymbolReady, ready)

	return state, output, nil
}

func ignitionRVOLScale(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	rvol := input.MustGet(SymbolRVOL)
	output := input
	output.Put(calculus.SymbolValue, rvol)
	output.Put(calculus.SymbolScale, 1)

	return state, output, nil
}

func ignitionRVOLLift(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	rvol := input.MustGet(SymbolRVOL)
	priorRVOL := number(state, SymbolIgnitionLastRVOL)
	lift := 0.0

	if priorRVOL > 0 {
		lift = rvol/priorRVOL - 1
	}

	output := input
	output.Put(SymbolRVOLLift, lift)
	output.Put(SymbolIgnitionLastRVOL, rvol)

	return state, output, nil
}

/*
ignitionDirectionalExhaustion composes adverse return, a causal return-scale
Ratio, positive volume fade, and Product. Alpha means exhaustion of an upward
move; beta is its reciprocal downward hypothesis.
*/
func ignitionDirectionalExhaustion(alpha bool) nomagique.Primitive {
	rejection, fade, decline, exhaustion := ignitionSideSymbols(alpha)

	return nomagique.Pipe(
		ignitionAdverseMove(alpha),
		calculus.Positive,
		ignitionMoveRatioOperands,
		calculus.Ratio,
		nomagique.Relay(calculus.SymbolResult, rejection),
		ignitionEvidenceScale(rejection),
		calculus.Squash,
		nomagique.Relay(calculus.SymbolResult, fade),
		ignitionVolumeDecrease,
		calculus.Difference,
		nomagique.Relay(calculus.SymbolResult, calculus.SymbolValue),
		calculus.Positive,
		ignitionVolumeDeclineRatio,
		calculus.Ratio,
		nomagique.Relay(calculus.SymbolResult, decline),
		ignitionExhaustionProduct(fade, decline),
		calculus.Product,
		nomagique.Relay(calculus.SymbolResult, exhaustion),
	)
}

func ignitionAdverseMove(alpha bool) nomagique.Primitive {
	return func(
		state nomagique.Frame,
		input nomagique.Frame,
	) (nomagique.Frame, nomagique.Frame, error) {
		move, found := input.Get(SymbolIgnitionReturn)

		if !found {
			return state, nomagique.Frame{}, fmt.Errorf(
				"ignition: log-return equation must precede exhaustion",
			)
		}

		if alpha {
			move = -move
		}

		output := input
		output.Put(calculus.SymbolValue, move)

		return state, output, nil
	}
}

func ignitionMoveRatioOperands(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	adverseMove := input.MustGet(calculus.SymbolResult)
	output := input
	output.Put(calculus.SymbolValue, adverseMove)
	output.Put(calculus.SymbolBaseline, input.MustGet(SymbolIgnitionMoveBaseline))
	output.Put(calculus.SymbolReady, input.MustGet(SymbolIgnitionMoveReady))

	return state, output, nil
}

func ignitionEvidenceScale(source nomagique.Symbol) nomagique.Primitive {
	return func(
		state nomagique.Frame,
		input nomagique.Frame,
	) (nomagique.Frame, nomagique.Frame, error) {
		output := input
		output.Put(calculus.SymbolValue, input.MustGet(source))
		output.Put(calculus.SymbolScale, 1)

		return state, output, nil
	}
}

func ignitionVolumeDecrease(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	output := input
	output.Put(calculus.SymbolLeft, number(state, SymbolIgnitionLastRVOL))
	output.Put(calculus.SymbolRight, input.MustGet(SymbolRVOL))

	return state, output, nil
}

func ignitionVolumeDeclineRatio(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	priorRVOL := number(state, SymbolIgnitionLastRVOL)
	output := input
	output.Put(calculus.SymbolValue, input.MustGet(calculus.SymbolResult))
	output.Put(calculus.SymbolBaseline, priorRVOL)
	output.Put(calculus.SymbolReady, boolNumber(priorRVOL > 0))

	return state, output, nil
}

func ignitionExhaustionProduct(
	fade nomagique.Symbol,
	decline nomagique.Symbol,
) nomagique.Primitive {
	return func(
		state nomagique.Frame,
		input nomagique.Frame,
	) (nomagique.Frame, nomagique.Frame, error) {
		output := input
		output.Put(calculus.SymbolLeft, input.MustGet(fade))
		output.Put(calculus.SymbolRight, input.MustGet(decline))

		return state, output, nil
	}
}

func ignitionSideSymbols(alpha bool) (
	rejection nomagique.Symbol,
	fade nomagique.Symbol,
	decline nomagique.Symbol,
	exhaustion nomagique.Symbol,
) {
	if alpha {
		return SymbolAlphaRejection, SymbolAlphaFade,
			SymbolAlphaDecline, SymbolAlphaExhaustion
	}

	return SymbolBetaRejection, SymbolBetaFade,
		SymbolBetaDecline, SymbolBetaExhaustion
}

func ignitionSeparation(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	output := input
	output.Put(
		SymbolIgnitionHypothesisSeparation,
		ignitionHypothesisSeparation(
			input.MustGet(SymbolAlphaExhaustion),
			input.MustGet(SymbolBetaExhaustion),
		),
	)
	output.Put(
		SymbolIgnitionClassified,
		boolNumber(
			input.MustGet(SymbolIgnitionRateReady) != 0 &&
				input.MustGet(SymbolIgnitionMoveReady) != 0,
		),
	)

	return state, output, nil
}

func ignitionHypothesisSeparation(alpha float64, beta float64) float64 {
	winner := math.Max(alpha, beta)

	if winner == 0 {
		return 0
	}

	return (winner - math.Min(alpha, beta)) / winner
}

/*
ignitionCommit appends the just-scored bar to causal history and starts the next
bar at the current print.
*/
func ignitionCommit(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	capacity := int(number(state, SymbolCapacity))
	barRate := input.MustGet(calculus.SymbolRate)
	priceMove := input.MustGet(SymbolIgnitionReturn)
	nextState := state
	nextState.Merge(input)

	if err := appendIgnitionHistory(
		&nextState,
		historyRates,
		capacity,
		barRate,
		true,
	); err != nil {
		return state, nomagique.Frame{}, err
	}

	if err := appendIgnitionHistory(
		&nextState,
		historyReturns,
		capacity,
		math.Abs(priceMove),
		false,
	); err != nil {
		return state, nomagique.Frame{}, err
	}

	nextState.Put(
		SymbolIgnitionObservedFromSec,
		number(nextState, SymbolIgnitionBarOpenSec),
	)
	nextState.Put(
		SymbolIgnitionObservedFromNsec,
		number(nextState, SymbolIgnitionBarOpenNsec),
	)
	nextState.Put(SymbolIgnitionBars, number(nextState, SymbolIgnitionBars)+1)
	nextState.Put(SymbolIgnitionPrevClose, number(nextState, SymbolLast))
	nextState.Put(SymbolIgnitionBarOpenSec, number(nextState, SymbolUnixSec))
	nextState.Put(SymbolIgnitionBarOpenNsec, number(nextState, SymbolUnixNsec))
	nextState.Put(SymbolIgnitionBarVolume, 0)

	return nextState, nextState, nil
}

func ignitionSquash(value float64, scale float64) float64 {
	input := nomagique.Frame{}
	input.Put(calculus.SymbolValue, value)
	input.Put(calculus.SymbolScale, scale)

	_, output, err := calculus.Squash(nomagique.Frame{}, input)

	if err != nil {
		panic(err)
	}

	return output.MustGet(calculus.SymbolResult)
}
