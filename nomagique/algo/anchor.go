package algo

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
)

var (
	SymbolVWAP                   = nomagique.MustIntern("anchor/vwap")
	SymbolAnchorLast             = nomagique.MustIntern("anchor/last")
	SymbolReportedVolume         = nomagique.MustIntern("anchor/reported_volume")
	SymbolAnchorFastHalflife     = nomagique.MustIntern("anchor/fast_halflife_sec")
	SymbolAnchorSlowHalflife     = nomagique.MustIntern("anchor/slow_halflife_sec")
	SymbolAnchorDispersionHalflife = nomagique.MustIntern(
		"anchor/dispersion_halflife_sec",
	)
	SymbolAnchorDetach           = nomagique.MustIntern("anchor/detach")
	SymbolAnchorLift             = nomagique.MustIntern("anchor/lift")
	SymbolAnchorFastBaseline     = nomagique.MustIntern("anchor/fast_baseline")
	SymbolAnchorSlowBaseline     = nomagique.MustIntern("anchor/slow_baseline")
	SymbolAnchorDispersion       = nomagique.MustIntern("anchor/dispersion")
	SymbolAnchorCount            = nomagique.MustIntern("anchor/count")
	SymbolAnchorLastSec          = nomagique.MustIntern("anchor/last_sec")
	SymbolAnchorLastNsec         = nomagique.MustIntern("anchor/last_nsec")
	SymbolAnchorMaturity         = nomagique.MustIntern("anchor/maturity")
	SymbolAnchorReady            = nomagique.MustIntern("anchor/ready")
)

/*
Anchor composes venue-reported ticker evidence. LogRatio measures last-price
detachment from VWAP, then evidence is scored against the prior slow anchor
before the fast, slow, and residual baselines observe the current point.
*/
func Anchor() nomagique.Primitive {
	return nomagique.Pipe(
		anchorObservation,
		calculus.LogRatio,
		nomagique.Relay(calculus.SymbolResult, SymbolAnchorDetach),
		anchorEvidence,
		anchorBaseline,
		anchorProject,
	)
}

func anchorObservation(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	last, hasLast := input.Get(SymbolLast)
	vwap, hasVWAP := input.Get(SymbolVWAP)
	reportedVolume, hasVolume := input.Get(SymbolReportedVolume)
	fastHalflife, hasFast := input.Get(SymbolAnchorFastHalflife)
	slowHalflife, hasSlow := input.Get(SymbolAnchorSlowHalflife)
	dispersionHalflife, hasDispersion := input.Get(SymbolAnchorDispersionHalflife)
	sec, hasSec := input.Get(SymbolUnixSec)
	nsec, hasNsec := input.Get(SymbolUnixNsec)

	if !hasLast || !hasVWAP || !hasVolume || !hasFast || !hasSlow ||
		!hasDispersion || !hasSec || !hasNsec {
		return state, nomagique.Frame{}, fmt.Errorf(
			"anchor: last, vwap, volume, halflives, and event time are required",
		)
	}

	if last <= 0 || vwap <= 0 || reportedVolume < 0 || fastHalflife <= 0 ||
		slowHalflife <= 0 || dispersionHalflife <= 0 || nsec < 0 || nsec >= 1e9 {
		return state, nomagique.Frame{}, fmt.Errorf(
			"anchor: prices and halflives must be positive, volume non-negative, and time normalized",
		)
	}

	previousSec, hasPreviousSec := state.Get(SymbolAnchorLastSec)
	previousNsec, hasPreviousNsec := state.Get(SymbolAnchorLastNsec)

	if hasPreviousSec && hasPreviousNsec &&
		elapsed(sec, nsec, previousSec, previousNsec) < 0 {
		return state, nomagique.Frame{}, fmt.Errorf(
			"anchor: event time must not regress",
		)
	}

	nextState := state
	nextState.Merge(input)
	nextState.Put(SymbolAnchorLast, last)
	nextState.Put(calculus.SymbolCurrent, last)
	nextState.Put(calculus.SymbolPrevious, vwap)

	return nextState, nextState, nil
}

func anchorEvidence(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	detach := input.MustGet(SymbolAnchorDetach)
	scale, hasScale := state.Get(SymbolAnchorSlowBaseline)
	count := number(state, SymbolAnchorCount)
	nextState := state
	nextState.Merge(input)
	nextState.Put(SymbolAlphaPrecursor, math.Max(detach, 0))
	nextState.Put(SymbolBetaPrecursor, math.Max(-detach, 0))
	nextState.Put(SymbolAnchorReady, 0)

	if count > 0 && scale > 0 && hasScale {
		nextState.Put(SymbolAnchorLift, math.Abs(detach)/scale-1)
		nextState.Put(
			SymbolAlphaPrecursorNormalized,
			ignitionSquash(math.Max(detach, 0), scale),
		)
		nextState.Put(
			SymbolBetaPrecursorNormalized,
			ignitionSquash(math.Max(-detach, 0), scale),
		)
		nextState.Put(SymbolAnchorReady, 1)
	}

	return nextState, nextState, nil
}

func anchorBaseline(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	detach := math.Abs(input.MustGet(SymbolAnchorDetach))
	sec := input.MustGet(SymbolUnixSec)
	nsec := input.MustGet(SymbolUnixNsec)
	previousSec, hasPreviousSec := state.Get(SymbolAnchorLastSec)
	previousNsec, hasPreviousNsec := state.Get(SymbolAnchorLastNsec)
	nextState := state
	nextState.Merge(input)

	if !hasPreviousSec || !hasPreviousNsec {
		nextState.Put(SymbolAnchorFastBaseline, detach)
		nextState.Put(SymbolAnchorSlowBaseline, detach)
		nextState.Put(SymbolAnchorDispersion, 0)
	} else {
		elapsedSeconds := elapsed(sec, nsec, previousSec, previousNsec)
		fast := anchorDecay(
			number(state, SymbolAnchorFastBaseline),
			detach,
			elapsedSeconds,
			input.MustGet(SymbolAnchorFastHalflife),
		)
		slowBefore := number(state, SymbolAnchorSlowBaseline)
		slow := anchorDecay(
			slowBefore,
			detach,
			elapsedSeconds,
			input.MustGet(SymbolAnchorSlowHalflife),
		)
		residual := detach - slowBefore
		energy := number(state, SymbolAnchorDispersion)
		energy *= energy
		alpha := 1 - math.Exp(
			-elapsedSeconds * math.Ln2 /
				input.MustGet(SymbolAnchorDispersionHalflife),
		)
		energy += alpha * (residual*residual - energy)
		nextState.Put(SymbolAnchorFastBaseline, fast)
		nextState.Put(SymbolAnchorSlowBaseline, slow)
		nextState.Put(SymbolAnchorDispersion, math.Sqrt(energy))
	}

	nextState.Put(SymbolAnchorLastSec, sec)
	nextState.Put(SymbolAnchorLastNsec, nsec)
	nextState.Put(SymbolAnchorCount, number(state, SymbolAnchorCount)+1)

	return nextState, nextState, nil
}

func anchorProject(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	nextState := state
	nextState.Merge(input)
	count := number(nextState, SymbolAnchorCount)
	nextState.Put(SymbolAnchorMaturity, count/(count+1))

	return nextState, nextState, nil
}

func anchorDecay(
	baseline float64,
	value float64,
	elapsedSeconds float64,
	halflife float64,
) float64 {
	alpha := 1 - math.Exp(-elapsedSeconds*math.Ln2/halflife)

	return baseline + alpha*(value-baseline)
}
