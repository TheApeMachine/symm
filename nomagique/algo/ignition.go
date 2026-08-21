package algo

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/logic"
)

const (
	MaxIgnitionHistory = nomagique.MaxSamples

	HistoryDeltas     = "deltas"
	HistoryRates      = "rates"
	HistoryMoves      = "moves"
	HistoryReturns    = "returns"
	HistoryPrecursors = "precursors"
)

const (
	historyDeltas = iota
	historyRates
	historyReturns
	historyFamilyCount
)

var (
	SymbolCapacity = nomagique.MustIntern("capacity")
	SymbolVolume   = nomagique.MustIntern("volume")
	SymbolLast     = nomagique.MustIntern("last")
	SymbolTradePrice = nomagique.MustIntern("trade/price")
	SymbolTradeQuantity = nomagique.MustIntern("trade/quantity")
	SymbolBid      = nomagique.MustIntern("bid")
	SymbolAsk      = nomagique.MustIntern("ask")

	SymbolIgnitionInitialized     = nomagique.MustIntern("window/initialized")
	SymbolIgnitionClassified      = nomagique.MustIntern("window/classified")
	SymbolIgnitionBars            = nomagique.MustIntern("window/bars")
	SymbolIgnitionHaveTime        = nomagique.MustIntern("window/have_time")
	SymbolIgnitionLastSec         = nomagique.MustIntern("window/last_sec")
	SymbolIgnitionLastNsec        = nomagique.MustIntern("window/last_nsec")
	SymbolIgnitionBarOpenSec      = nomagique.MustIntern("window/bar_open_sec")
	SymbolIgnitionBarOpenNsec     = nomagique.MustIntern("window/bar_open_nsec")
	SymbolIgnitionObservedFromSec = nomagique.MustIntern(
		"window/observed_from_sec",
	)
	SymbolIgnitionObservedFromNsec = nomagique.MustIntern(
		"window/observed_from_nsec",
	)
	SymbolIgnitionBarVolume = nomagique.MustIntern("window/bar_volume")
	SymbolIgnitionPrevClose = nomagique.MustIntern("window/previous_close")
	SymbolIgnitionLastRVOL  = nomagique.MustIntern("window/previous_rvol")
	SymbolIgnitionBarClosed = nomagique.MustIntern("window/bar_closed")
	SymbolIgnitionMaturity  = nomagique.MustIntern("ignition/maturity")

	SymbolRVOL                         = nomagique.MustIntern("rvol")
	SymbolRVOLNormalized               = nomagique.MustIntern("rvol/normalized")
	SymbolRVOLLift                     = nomagique.MustIntern("rvol/lift")
	SymbolMidpoint                     = nomagique.MustIntern("midpoint")
	SymbolSpread                       = nomagique.MustIntern("spread")
	SymbolSpreadNormalized             = nomagique.MustIntern("spread/normalized")
	SymbolSpreadBaseline               = nomagique.MustIntern("spread/baseline")
	SymbolCompression                  = nomagique.MustIntern("compression")
	SymbolMaturity                     = nomagique.MustIntern("maturity")
	SymbolIgnitionHypothesisSeparation = nomagique.MustIntern(
		"ignition/hypothesis_separation",
	)

	SymbolIgnitionBarRate      = nomagique.MustIntern("window/bar_rate")
	SymbolIgnitionReturn       = nomagique.MustIntern("window/log_return")
	SymbolIgnitionRateBaseline = nomagique.MustIntern("window/rate_baseline")

	SymbolAlphaRVOL                = nomagique.MustIntern("alpha/rvol")
	SymbolAlphaPrecursor           = nomagique.MustIntern("alpha/precursor")
	SymbolAlphaPrecursorNormalized = nomagique.MustIntern(
		"alpha/precursor/normalized",
	)
	SymbolAlphaExhaustion = nomagique.MustIntern("alpha/exhaustion")

	SymbolBetaRVOL                = nomagique.MustIntern("beta/rvol")
	SymbolBetaPrecursor           = nomagique.MustIntern("beta/precursor")
	SymbolBetaPrecursorNormalized = nomagique.MustIntern(
		"beta/precursor/normalized",
	)
	SymbolBetaExhaustion = nomagique.MustIntern("beta/exhaustion")
)

/*
NewIgnitionState returns an empty universal state.
*/
func NewIgnitionState() nomagique.Frame {
	return nomagique.Frame{}
}

/*
Ignition composes the executed-trade volume clock. A trade first advances the
data-derived bar. Only a closed bar is routed through the universal Rate and
LogRatio atoms; the resulting tape observation is scored against causal
history and committed. Book geometry and venue anchors are deliberately not
part of this algorithm.
*/
func Ignition() nomagique.Primitive {
	return nomagique.Pipe(
		ignitionVolumeClock,
		logic.If(
			ignitionBarClosed,
			nomagique.Pipe(
				nomagique.Fork(calculus.Rate, calculus.LogRatio),
				nomagique.Relay(calculus.SymbolResult, SymbolIgnitionReturn),
				ignitionBaselines,
				ignitionRelativeVolume(),
				nomagique.Fork(
					ignitionDirectionalExhaustion(sideAlpha),
					ignitionDirectionalExhaustion(sideBeta),
				),
				ignitionSeparation,
				ignitionCommit,
			),
			nomagique.Identity,
		),
		ignitionProject,
	)
}

/*
ignitionVolumeClock advances the data-derived volume bar. A closed bar exposes
the operands consumed by calculus.Rate and calculus.LogRatio.
*/
func ignitionVolumeClock(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	capacity, volume, last, sec, nsec, hasTime, err := ignitionObservation(&input)

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	nextState := state
	copyIgnitionObservation(&nextState, &input)
	nextState.Put(SymbolIgnitionBarClosed, 0)

	if number(nextState, SymbolIgnitionInitialized) == 0 {
		err = initializeIgnition(
			&nextState,
			capacity,
			volume,
			last,
			sec,
			nsec,
			hasTime,
		)
	} else {
		err = advanceIgnition(
			&nextState,
			capacity,
			volume,
			last,
			sec,
			nsec,
			hasTime,
		)
	}

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	return nextState, nextState, nil
}

/*
ignitionBarClosed exposes the volume clock's explicit routing condition.
*/
func ignitionBarClosed(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	closed, found := input.Get(SymbolIgnitionBarClosed)

	if !found {
		return state, nomagique.Frame{}, fmt.Errorf(
			"ignition: volume clock did not report bar state",
		)
	}

	output := input
	output.Put(logic.SymbolCondition, closed)

	return state, output, nil
}

func ignitionObservation(
	input *nomagique.Frame,
) (
	capacity int,
	volume float64,
	last float64,
	sec float64,
	nsec float64,
	hasTime bool,
	err error,
) {
	capacityValue, found := input.Get(SymbolCapacity)

	if !found || capacityValue <= 0 || capacityValue != math.Trunc(capacityValue) ||
		capacityValue > MaxIgnitionHistory || !finite(capacityValue) {
		err = fmt.Errorf(
			"ignition: capacity must be an integer from 1 through %d",
			MaxIgnitionHistory,
		)
		return
	}

	capacity = int(capacityValue)
	volume, found = input.Get(SymbolVolume)

	if !found || volume <= 0 || !finite(volume) {
		err = fmt.Errorf("ignition: trade volume must be finite and positive")
		return
	}

	last, found = input.Get(SymbolLast)

	if !found || last <= 0 || !finite(last) {
		err = fmt.Errorf("ignition: trade price must be finite and positive")
		return
	}

	sec, hasSec := input.Get(SymbolUnixSec)
	nsec, hasNsec := input.Get(SymbolUnixNsec)

	if hasSec != hasNsec {
		err = fmt.Errorf("ignition: timestamp requires unix_sec and unix_nsec")
		return
	}

	if hasSec {
		if !finite(sec, nsec) || nsec < 0 || nsec >= 1e9 {
			err = fmt.Errorf(
				"ignition: timestamp coordinates must be finite and normalized",
			)
			return
		}

		hasTime = sec != 0 || nsec != 0
	}

	return
}

func copyIgnitionObservation(state *nomagique.Frame, input *nomagique.Frame) {
	for _, symbol := range []nomagique.Symbol{
		SymbolCapacity,
		SymbolVolume,
		SymbolLast,
		SymbolTradePrice,
		SymbolTradeQuantity,
		SymbolUnixSec,
		SymbolUnixNsec,
	} {
		value, found := input.Get(symbol)

		if found {
			state.Put(symbol, value)
			continue
		}

		state.Delete(symbol)
	}
}

func initializeIgnition(
	state *nomagique.Frame,
	capacity int,
	volume float64,
	last float64,
	sec float64,
	nsec float64,
	hasTime bool,
) error {
	state.Put(SymbolIgnitionInitialized, 1)
	state.Put(SymbolIgnitionClassified, 0)
	state.Put(SymbolIgnitionBars, 0)
	state.Put(SymbolIgnitionPrevClose, last)
	state.Put(SymbolIgnitionLastRVOL, 0)
	state.Put(SymbolIgnitionBarVolume, volume)
	initializeIgnitionOutput(state)

	if hasTime {
		state.Put(SymbolIgnitionHaveTime, 1)
		state.Put(SymbolIgnitionLastSec, sec)
		state.Put(SymbolIgnitionLastNsec, nsec)
		state.Put(SymbolIgnitionBarOpenSec, sec)
		state.Put(SymbolIgnitionBarOpenNsec, nsec)
		state.Put(SymbolIgnitionObservedFromSec, sec)
		state.Put(SymbolIgnitionObservedFromNsec, nsec)
	}

	return appendIgnitionHistory(state, historyDeltas, capacity, volume, true)
}

func initializeIgnitionOutput(state *nomagique.Frame) {
	for _, symbol := range []nomagique.Symbol{
		SymbolRVOL,
		SymbolRVOLNormalized,
		SymbolRVOLLift,
		SymbolIgnitionBarRate,
		SymbolIgnitionRateBaseline,
		SymbolReady,
		SymbolMaturity,
		SymbolIgnitionMaturity,
		SymbolIgnitionHypothesisSeparation,
		SymbolAlphaRVOL,
		SymbolAlphaExhaustion,
		SymbolBetaRVOL,
		SymbolBetaExhaustion,
	} {
		state.Put(symbol, 0)
	}
}

func advanceIgnition(
	state *nomagique.Frame,
	capacity int,
	volume float64,
	last float64,
	sec float64,
	nsec float64,
	hasTime bool,
) error {
	windowHasTime := number(*state, SymbolIgnitionHaveTime) != 0

	if windowHasTime && hasTime && before(
		sec,
		nsec,
		number(*state, SymbolIgnitionLastSec),
		number(*state, SymbolIgnitionLastNsec),
	) {
		return fmt.Errorf("ignition: observation time cannot move backwards")
	}

	if hasTime {
		state.Put(SymbolIgnitionLastSec, sec)
		state.Put(SymbolIgnitionLastNsec, nsec)

		if !windowHasTime {
			windowHasTime = true
			state.Put(SymbolIgnitionHaveTime, 1)
			state.Put(SymbolIgnitionBarOpenSec, sec)
			state.Put(SymbolIgnitionBarOpenNsec, nsec)
			state.Put(SymbolIgnitionObservedFromSec, sec)
			state.Put(SymbolIgnitionObservedFromNsec, nsec)
		}
	}

	barVolume := number(*state, SymbolIgnitionBarVolume) + volume
	state.Put(SymbolIgnitionBarVolume, barVolume)
	barTarget, targetReady, err := ignitionHistoryMedian(state, historyDeltas)

	if err != nil {
		return err
	}

	barOpenSec := number(*state, SymbolIgnitionBarOpenSec)
	barOpenNsec := number(*state, SymbolIgnitionBarOpenNsec)
	closes := targetReady && barTarget > 0 && barVolume >= barTarget &&
		windowHasTime && hasTime && after(sec, nsec, barOpenSec, barOpenNsec)

	if closes {
		previousClose := number(*state, SymbolIgnitionPrevClose)

		if previousClose <= 0 {
			return fmt.Errorf("ignition: previous close must be positive")
		}

		state.Put(SymbolIgnitionBarClosed, 1)
		state.Put(calculus.SymbolCount, barVolume)
		state.Put(calculus.SymbolDuration, elapsed(
			sec,
			nsec,
			barOpenSec,
			barOpenNsec,
		))
		state.Put(calculus.SymbolCurrent, last)
		state.Put(calculus.SymbolPrevious, previousClose)
	}

	return appendIgnitionHistory(state, historyDeltas, capacity, volume, true)
}

/*
ignitionProject derives public readiness and maturity from committed tape
state.
*/
func ignitionProject(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	nextState := state
	nextState.Merge(input)
	ready := number(nextState, SymbolIgnitionClassified)
	nextState.Put(SymbolReady, boolNumber(ready != 0))
	bars := number(nextState, SymbolIgnitionBars)
	maturity := bars / (bars + 1)
	nextState.Put(SymbolMaturity, maturity)
	nextState.Put(SymbolIgnitionMaturity, maturity)

	return nextState, nextState, nil
}

func after(sec float64, nsec float64, otherSec float64, otherNsec float64) bool {
	return sec > otherSec || sec == otherSec && nsec > otherNsec
}

func boolNumber(value bool) float64 {
	if value {
		return 1
	}

	return 0
}
