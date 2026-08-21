package algo

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/logic"
)

const (
	PumpDumpEventTrade = iota + 1
	PumpDumpEventTicker
	PumpDumpEventLevel3
)

var (
	SymbolPumpDumpEvent            = nomagique.MustIntern("pumpdump/event")
	SymbolPumpDumpObservedFromSec  = nomagique.MustIntern("pumpdump/observed_from_sec")
	SymbolPumpDumpObservedFromNsec = nomagique.MustIntern("pumpdump/observed_from_nsec")
)

/*
PumpDump is the switchable-schema composition for one symbol. Each event is
routed to exactly one authoritative estimator: trades to Ignition, tickers to
Anchor, and committed resident Level 3 books to Ladder. The final projection
joins their retained numeric state without a local cache or snapshot.
*/
func PumpDump() nomagique.Primitive {
	return nomagique.Pipe(
		logic.Circuit(
			[]logic.Rule{
				{
					When: pumpDumpEvent(PumpDumpEventTrade),
					Then: Ignition(),
				},
				{
					When: pumpDumpEvent(PumpDumpEventTicker),
					Then: Anchor(),
				},
				{
					When: pumpDumpEvent(PumpDumpEventLevel3),
					Then: Ladder(),
				},
			},
			pumpDumpUnknownEvent,
		),
		pumpDumpProject,
	)
}

func pumpDumpEvent(expected int) nomagique.Primitive {
	return func(
		state nomagique.Frame,
		input nomagique.Frame,
	) (nomagique.Frame, nomagique.Frame, error) {
		event, found := input.Get(SymbolPumpDumpEvent)

		if !found {
			return state, nomagique.Frame{}, fmt.Errorf(
				"pumpdump: event tag is required",
			)
		}

		output := input
		output.Put(logic.SymbolCondition, boolNumber(event == float64(expected)))

		return state, output, nil
	}
}

func pumpDumpUnknownEvent(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	return state, nomagique.Frame{}, fmt.Errorf(
		"pumpdump: unsupported event tag %v",
		input.MustGet(SymbolPumpDumpEvent),
	)
}

func pumpDumpProject(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	nextState := state
	nextState.Merge(input)
	pumpDumpBookProjection(&nextState)
	event := int(nextState.MustGet(SymbolPumpDumpEvent))
	observedFromSec := nextState.MustGet(SymbolUnixSec)
	observedFromNsec := nextState.MustGet(SymbolUnixNsec)

	if event == PumpDumpEventTrade {
		observedFromSec = number(nextState, SymbolIgnitionObservedFromSec)
		observedFromNsec = number(nextState, SymbolIgnitionObservedFromNsec)
	}

	nextState.Put(SymbolPumpDumpObservedFromSec, observedFromSec)
	nextState.Put(SymbolPumpDumpObservedFromNsec, observedFromNsec)
	ignitionMaturity := number(nextState, SymbolIgnitionMaturity)
	anchorMaturity := number(nextState, SymbolAnchorMaturity)
	ladderMaturity := number(nextState, SymbolLadderMaturity)
	nextState.Put(
		SymbolMaturity,
		math.Min(ignitionMaturity, math.Min(anchorMaturity, ladderMaturity)),
	)
	ready := number(nextState, SymbolIgnitionClassified) != 0 &&
		number(nextState, SymbolAnchorReady) != 0 &&
		number(nextState, SymbolLadderReady) != 0
	nextState.Put(SymbolReady, boolNumber(ready))
	nextState.Put(
		SymbolIgnitionHypothesisSeparation,
		ignitionHypothesisSeparation(
			number(nextState, SymbolAlphaExhaustion),
			number(nextState, SymbolBetaExhaustion),
		),
	)

	return nextState, nextState, nil
}

func pumpDumpBookProjection(state *nomagique.Frame) {
	spread, hasSpread := state.Get(SymbolLadderSpread)
	midpoint, hasMidpoint := state.Get(SymbolMidpoint)

	if hasSpread {
		state.Put(SymbolSpread, spread)
	}

	if hasSpread && hasMidpoint && midpoint > 0 {
		state.Put(SymbolSpreadNormalized, spread/midpoint)
	}

	if baseline, found := state.Get(SymbolLadderSpreadBaseline); found {
		state.Put(SymbolSpreadBaseline, baseline)
	}

	if compression, found := state.Get(SymbolLadderCompression); found {
		state.Put(SymbolCompression, compression)
	}
}
