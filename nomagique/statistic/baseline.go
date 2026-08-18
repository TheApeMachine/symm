package statistic

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique"
)

var (
	SymbolUnixSec  = nomagique.MustIntern("unix_sec")
	SymbolUnixNsec = nomagique.MustIntern("unix_nsec")

	SymbolBaselineValue       = nomagique.MustIntern("baseline/value")
	SymbolBaselineEfficiency  = nomagique.MustIntern("baseline/efficiency")
	SymbolBaselineWindow      = nomagique.MustIntern("baseline/effective_window")
	SymbolBaselineLastSec     = nomagique.MustIntern("baseline/last_sec")
	SymbolBaselineLastNsec    = nomagique.MustIntern("baseline/last_nsec")
	SymbolBaselineFastHalflife = nomagique.MustIntern("baseline/fast_halflife_sec")
	SymbolBaselineSlowHalflife = nomagique.MustIntern("baseline/slow_halflife_sec")
)

/*
Baseline is the adaptive central estimate over a window's retained samples.
Its sharpness is not configured: the series' own efficiency ratio — net
displacement over path length across the retained ring — interpolates the
adaptation halflife between the composed fast and slow horizons, so a direct,
low-noise series is tracked closely and a choppy one forces the effective
window wide. The first observation seeds the baseline with itself, and the
effective window is always emitted so the adaptation the series demanded is
visible rather than hidden in a constant.
*/
func Baseline(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	value, hasValue := input.Get(nomagique.SampleValue)
	fastHalflife, hasFast := input.Get(SymbolBaselineFastHalflife)
	slowHalflife, hasSlow := input.Get(SymbolBaselineSlowHalflife)
	sec, hasSec := input.Get(SymbolUnixSec)
	nsec, hasNsec := input.Get(SymbolUnixNsec)

	if !hasValue || !hasFast || !hasSlow || !hasSec || !hasNsec {
		return state, nomagique.Frame{}, fmt.Errorf(
			"statistic: baseline requires a value, both halflife horizons, and event time",
		)
	}

	if fastHalflife <= 0 || slowHalflife <= 0 || fastHalflife > slowHalflife ||
		nsec < 0 || nsec >= 1e9 {
		return state, nomagique.Frame{}, fmt.Errorf(
			"statistic: baseline requires positive halflife horizons with fast at most slow, and normalized nanoseconds",
		)
	}

	count := windowSampleCount(state)

	if count < 1 {
		return state, nomagique.Frame{}, fmt.Errorf(
			"statistic: baseline requires the window's retained samples",
		)
	}

	previousSec, hasLastSec := state.Get(SymbolBaselineLastSec)
	previousNsec, hasLastNsec := state.Get(SymbolBaselineLastNsec)

	if hasLastSec && hasLastNsec {
		if elapsedSince(sec, nsec, previousSec, previousNsec) < 0 {
			return state, nomagique.Frame{}, fmt.Errorf(
				"statistic: baseline event time must not regress",
			)
		}
	}

	nextState := state

	_, hasBaseline := state.Get(SymbolBaselineValue)

	if !hasBaseline || !hasLastSec || !hasLastNsec {
		nextState.Put(SymbolBaselineValue, value)
		nextState.Put(SymbolBaselineEfficiency, 0)
		nextState.Put(SymbolBaselineWindow, 1)
		nextState.Put(SymbolBaselineLastSec, sec)
		nextState.Put(SymbolBaselineLastNsec, nsec)

		return nextState, baselineOutput(nextState, value), nil
	}

	efficiency := windowEfficiency(state, count)
	halflife := slowHalflife + efficiency*(fastHalflife-slowHalflife)
	elapsed := elapsedSince(sec, nsec, previousSec, previousNsec)
	alpha := 1 - math.Exp(-elapsed*math.Ln2/halflife)

	baseline, _ := state.Get(SymbolBaselineValue)
	baseline += alpha * (value - baseline)

	nextState.Put(SymbolBaselineValue, baseline)
	nextState.Put(SymbolBaselineEfficiency, efficiency)
	nextState.Put(SymbolBaselineWindow, 2/alpha-1)
	nextState.Put(SymbolBaselineLastSec, sec)
	nextState.Put(SymbolBaselineLastNsec, nsec)

	return nextState, baselineOutput(nextState, value), nil
}

func baselineOutput(state nomagique.Frame, value float64) nomagique.Frame {
	output := nomagique.Frame{}
	output.Put(nomagique.SampleValue, value)

	baseline, _ := state.Get(SymbolBaselineValue)
	efficiency, _ := state.Get(SymbolBaselineEfficiency)
	window, _ := state.Get(SymbolBaselineWindow)

	output.Put(SymbolBaselineValue, baseline)
	output.Put(SymbolBaselineEfficiency, efficiency)
	output.Put(SymbolBaselineWindow, window)
	output.Put(SymbolReady, 1)

	return output
}

/*
windowEfficiency reports the retained ring's efficiency ratio: the absolute
net displacement between its oldest and newest samples over the path length
of every consecutive step, walked in arrival order. A flat ring has no
direction and reports zero, holding the baseline at its slowest.
*/
func windowEfficiency(state nomagique.Frame, count int) float64 {
	if count < 2 {
		return 0
	}

	capacity, _ := state.Get(nomagique.MustIntern("capacity"))
	head := windowSampleHead(state)
	slots := make([]float64, 0, count)

	for index := range count {
		sample, _ := state.Get(nomagique.MustSampleSymbol((head + index) % int(capacity)))
		slots = append(slots, sample)
	}

	displacement := math.Abs(slots[len(slots)-1] - slots[0])
	path := 0.0

	for index := 1; index < len(slots); index++ {
		path += math.Abs(slots[index] - slots[index-1])
	}

	if path <= 0 {
		return 0
	}

	return displacement / path
}

func windowSampleCount(state nomagique.Frame) int {
	value, found := state.Get(nomagique.SampleCount)

	if !found {
		return 0
	}

	return int(value)
}

func windowSampleHead(state nomagique.Frame) int {
	value, found := state.Get(nomagique.SampleHead)

	if !found {
		return 0
	}

	return int(value)
}

func elapsedSince(
	sec float64,
	nsec float64,
	previousSec float64,
	previousNsec float64,
) float64 {
	return sec - previousSec + (nsec-previousNsec)*1e-9
}
