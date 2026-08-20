package algo

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique"
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
	historyPrecursors
	historyFamilyCount
)

var (
	SymbolCapacity = nomagique.MustIntern("capacity")
	SymbolVolume   = nomagique.MustIntern("volume")
	SymbolLast     = nomagique.MustIntern("last")
	SymbolBid      = nomagique.MustIntern("bid")
	SymbolAsk      = nomagique.MustIntern("ask")

	SymbolIgnitionInitialized = nomagique.MustIntern("window/initialized")
	SymbolIgnitionClassified  = nomagique.MustIntern("window/classified")
	SymbolIgnitionBars        = nomagique.MustIntern("window/bars")
	SymbolIgnitionHaveTime    = nomagique.MustIntern("window/have_time")
	SymbolIgnitionLastSec     = nomagique.MustIntern("window/last_sec")
	SymbolIgnitionLastNsec    = nomagique.MustIntern("window/last_nsec")
	SymbolIgnitionBarOpenSec  = nomagique.MustIntern("window/bar_open_sec")
	SymbolIgnitionBarOpenNsec = nomagique.MustIntern("window/bar_open_nsec")
	SymbolIgnitionBarVolume   = nomagique.MustIntern("window/bar_volume")
	SymbolIgnitionPrevClose   = nomagique.MustIntern("window/previous_close")
	SymbolIgnitionLastRVOL    = nomagique.MustIntern("window/previous_rvol")

	SymbolRVOL     = nomagique.MustIntern("rvol")
	SymbolSpread   = nomagique.MustIntern("spread")
	SymbolMaturity = nomagique.MustIntern("maturity")

	SymbolIgnitionBarRate     = nomagique.MustIntern("window/bar_rate")
	SymbolIgnitionRateBaseline = nomagique.MustIntern("window/rate_baseline")

	SymbolAlphaRVOL       = nomagique.MustIntern("alpha/rvol")
	SymbolAlphaPrecursor  = nomagique.MustIntern("alpha/precursor")
	SymbolAlphaExhaustion = nomagique.MustIntern("alpha/exhaustion")

	SymbolBetaRVOL       = nomagique.MustIntern("beta/rvol")
	SymbolBetaPrecursor  = nomagique.MustIntern("beta/precursor")
	SymbolBetaExhaustion = nomagique.MustIntern("beta/exhaustion")
)

/*
NewIgnitionState returns an empty universal state. The first valid observation
initializes the causal volume clock and all public output slots.
*/
func NewIgnitionState() nomagique.Frame {
	return nomagique.Frame{}
}

/*
Ignition advances one ordered market stream through a causal volume clock. Use
one nomagique.Stream per key; the reducer itself contains no keyed or domain
state outside its Frame.
*/
/*
Ignition is a composite Primitive: the causal volume-clock transition runs
first, and the classified output projection runs second. Both steps are plain
primitives; the score path already composes the shared calculus.Ratio and
calculus.Squash atoms through ignitionScore.
*/
func Ignition() nomagique.Primitive {
	return nomagique.Pipe(ignitionTransition, ignitionClassify)
}

/*
ignitionTransition advances one ordered market stream through the causal
volume clock: it validates the observation, initializes or advances the
current bar, and, when the bar's volume reaches its history median target,
closes it and scores the tape.
*/
func ignitionTransition(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	capacity, volume, last, bid, ask, sec, nsec, hasTime, err := ignitionObservation(&input)

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	nextState := state
	copyIgnitionObservation(&nextState, &input)
	spread := ask - bid

	if number(nextState, SymbolIgnitionInitialized) == 0 {
		if err := initializeIgnition(
			&nextState,
			capacity,
			volume,
			last,
			sec,
			nsec,
			hasTime,
		); err != nil {
			return state, nomagique.Frame{}, err
		}
	} else if err := advanceIgnition(
		&nextState,
		capacity,
		volume,
		last,
		sec,
		nsec,
		hasTime,
	); err != nil {
		return state, nomagique.Frame{}, err
	}

	nextState.Put(SymbolSpread, spread)

	return nextState, nextState, nil
}

/*
ignitionClassify projects the transition's state into the public output slots:
spread, readiness, and bar-driven maturity.
*/
func ignitionClassify(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	nextState := state
	spread := number(nextState, SymbolSpread)
	composeIgnition(&nextState, spread)

	return nextState, nextState, nil
}

func ignitionObservation(
	input *nomagique.Frame,
) (
	capacity int,
	volume float64,
	last float64,
	bid float64,
	ask float64,
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
	values := []struct {
		symbol nomagique.Symbol
		target *float64
	}{
		{symbol: SymbolVolume, target: &volume},
		{symbol: SymbolLast, target: &last},
		{symbol: SymbolBid, target: &bid},
		{symbol: SymbolAsk, target: &ask},
	}

	for _, required := range values {
		value, present := input.Get(required.symbol)

		if !present || value <= 0 || !finite(value) {
			err = fmt.Errorf(
				"ignition: volume, last, bid, and ask must be finite and positive",
			)
			return
		}

		*required.target = value
	}

	if ask <= bid {
		err = fmt.Errorf("ignition: ask must be above bid")
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
		SymbolBid,
		SymbolAsk,
		SymbolUnixSec,
		SymbolUnixNsec,
	} {
		value, found := input.Get(symbol)

		if found {
			state.Put(symbol, value)
		} else {
			state.Delete(symbol)
		}
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
	}

	return appendIgnitionHistory(state, historyDeltas, capacity, volume, true)
}

func initializeIgnitionOutput(state *nomagique.Frame) {
	for _, symbol := range []nomagique.Symbol{
		SymbolRVOL,
		SymbolSpread,
		SymbolIgnitionBarRate,
		SymbolIgnitionRateBaseline,
		SymbolReady,
		SymbolMaturity,
		SymbolAlphaRVOL,
		SymbolAlphaPrecursor,
		SymbolAlphaExhaustion,
		SymbolBetaRVOL,
		SymbolBetaPrecursor,
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
		if err := closeIgnitionBar(
			state,
			capacity,
			last,
			sec,
			nsec,
			barVolume,
		); err != nil {
			return err
		}
	}

	return appendIgnitionHistory(state, historyDeltas, capacity, volume, true)
}

func closeIgnitionBar(
	state *nomagique.Frame,
	capacity int,
	last float64,
	sec float64,
	nsec float64,
	barVolume float64,
) error {
	previousClose := number(*state, SymbolIgnitionPrevClose)

	if previousClose <= 0 {
		return fmt.Errorf("ignition: previous close must be positive")
	}

	priceMove := math.Log(last / previousClose)
	duration := elapsed(
		sec,
		nsec,
		number(*state, SymbolIgnitionBarOpenSec),
		number(*state, SymbolIgnitionBarOpenNsec),
	)

	if duration <= 0 {
		return fmt.Errorf("ignition: volume bar requires positive elapsed event time")
	}

	barRate := barVolume / duration

	if !finite(priceMove, barRate) {
		return fmt.Errorf("ignition: calculated bar values must be finite")
	}

	if err := scoreIgnition(state, barRate, priceMove); err != nil {
		return err
	}

	if err := appendIgnitionHistory(state, historyRates, capacity, barRate, true); err != nil {
		return err
	}

	if err := appendIgnitionHistory(state, historyReturns, capacity, math.Abs(priceMove), false); err != nil {
		return err
	}

	if err := appendIgnitionHistory(state, historyPrecursors, capacity, math.Abs(priceMove), true); err != nil {
		return err
	}

	state.Put(SymbolIgnitionBars, number(*state, SymbolIgnitionBars)+1)
	state.Put(SymbolIgnitionPrevClose, last)
	state.Put(SymbolIgnitionBarOpenSec, sec)
	state.Put(SymbolIgnitionBarOpenNsec, nsec)
	state.Put(SymbolIgnitionBarVolume, 0)

	return nil
}

func composeIgnition(state *nomagique.Frame, spread float64) {
	state.Put(SymbolSpread, spread)
	ready := number(*state, SymbolIgnitionClassified)
	state.Put(SymbolReady, boolNumber(ready != 0))
	bars := number(*state, SymbolIgnitionBars)
	maturity := 0.0

	if bars >= 0 {
		maturity = bars / (bars + 1)
	}

	state.Put(SymbolMaturity, maturity)
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
