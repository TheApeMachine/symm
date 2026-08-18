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

/*
scoreIgnition derives the tape metrics for one closed volume bar: relative
volume against the tape's own median rate baseline, the per-side price
precursor against the precursor baseline, and per-side exhaustion from a
declining rate carrying adverse rejection. Rejection is already normalized by
the move baseline, so it squashes against the unit of that normalization.
The tape measures and stores; fusion, winning sides, and categories are
downstream decisions that do not belong here.
*/
func scoreIgnition(
	state *nomagique.Frame,
	barRate float64,
	priceMove float64,
) error {
	rateBaseline, rateReady, err := ignitionHistoryMedian(state, historyRates)

	if err != nil {
		return err
	}

	precursorBaseline, precursorReady, err := ignitionHistoryMedian(
		state,
		historyPrecursors,
	)

	if err != nil {
		return err
	}

	moveBaseline, moveReady, err := ignitionHistoryMedian(state, historyReturns)

	if err != nil {
		return err
	}

	rvol := ignitionRatio(barRate, rateBaseline, rateReady)
	alphaMove := math.Max(priceMove, 0)
	betaMove := math.Max(-priceMove, 0)
	alphaPrecursor := ignitionRatio(alphaMove, precursorBaseline, precursorReady)
	betaPrecursor := ignitionRatio(betaMove, precursorBaseline, precursorReady)
	alphaRejection := ignitionRatio(betaMove, moveBaseline, moveReady)
	betaRejection := ignitionRatio(alphaMove, moveBaseline, moveReady)

	priorRVOL := number(*state, SymbolIgnitionLastRVOL)
	alphaExhaustion := ignitionExhaustion(priorRVOL, rvol, alphaRejection)
	betaExhaustion := ignitionExhaustion(priorRVOL, rvol, betaRejection)

	if !finite(rvol, alphaPrecursor, betaPrecursor) {
		return fmt.Errorf("ignition: calculated tape metrics must be finite")
	}

	putIgnitionSide(state, sideAlpha, rvol, alphaPrecursor, alphaExhaustion)
	putIgnitionSide(state, sideBeta, rvol, betaPrecursor, betaExhaustion)

	state.Put(SymbolRVOL, rvol)
	state.Put(SymbolIgnitionBarRate, barRate)
	state.Put(SymbolIgnitionRateBaseline, rateBaseline)
	state.Put(SymbolIgnitionLastRVOL, rvol)
	state.Put(SymbolIgnitionClassified, boolNumber(rateReady && moveReady))

	return nil
}

/*
ignitionExhaustion reports how much the rate has declined from its prior
reading, scaled by squashed adverse rejection.
*/
func ignitionExhaustion(
	priorRVOL float64,
	rvol float64,
	rejection float64,
) float64 {
	positiveDecrease := math.Max(priorRVOL-rvol, 0)
	relativeDecrease := ignitionRatio(positiveDecrease, priorRVOL, priorRVOL > 0)

	return relativeDecrease * ignitionSquash(rejection, 1)
}

func ignitionRatio(value float64, baseline float64, ready bool) float64 {
	input := nomagique.Frame{}
	input.Put(calculus.SymbolValue, value)
	input.Put(calculus.SymbolBaseline, baseline)
	input.Put(calculus.SymbolReady, boolNumber(ready))

	_, output, err := calculus.Ratio(nomagique.Frame{}, input)

	if err != nil {
		panic(err)
	}

	result, _ := output.Get(calculus.SymbolResult)

	return result
}

func ignitionSquash(value float64, scale float64) float64 {
	input := nomagique.Frame{}
	input.Put(calculus.SymbolValue, value)
	input.Put(calculus.SymbolScale, scale)

	_, output, err := calculus.Squash(nomagique.Frame{}, input)

	if err != nil {
		panic(err)
	}

	result, _ := output.Get(calculus.SymbolResult)

	return result
}

func putIgnitionSide(
	state *nomagique.Frame,
	alpha bool,
	rvol float64,
	precursor float64,
	exhaustion float64,
) {
	if alpha {
		state.Put(SymbolAlphaRVOL, rvol)
		state.Put(SymbolAlphaPrecursor, precursor)
		state.Put(SymbolAlphaExhaustion, exhaustion)

		return
	}

	state.Put(SymbolBetaRVOL, rvol)
	state.Put(SymbolBetaPrecursor, precursor)
	state.Put(SymbolBetaExhaustion, exhaustion)
}
