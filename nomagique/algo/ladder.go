package algo

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique"
)

var (
	SymbolLadderBidDepth = nomagique.MustIntern("ladder/bid_depth")
	SymbolLadderAskDepth = nomagique.MustIntern("ladder/ask_depth")
	SymbolLadderSpread   = nomagique.MustIntern("ladder/spread")
	SymbolLadderBidDelta = nomagique.MustIntern("ladder/bid_delta")
	SymbolLadderAskDelta = nomagique.MustIntern("ladder/ask_delta")
	SymbolLadderHalflife = nomagique.MustIntern("ladder/halflife_sec")
	SymbolLadderEpsilon  = nomagique.MustIntern("ladder/epsilon")

	SymbolLadderImbalance      = nomagique.MustIntern("ladder/imbalance")
	SymbolLadderCompression    = nomagique.MustIntern("ladder/compression")
	SymbolLadderSpreadTightening = nomagique.MustIntern(
		"ladder/spread_tightening",
	)
	SymbolLadderSpreadDeviation = nomagique.MustIntern(
		"ladder/spread_deviation",
	)
	SymbolLadderBidDepletion   = nomagique.MustIntern("ladder/bid_depletion")
	SymbolLadderAskDepletion   = nomagique.MustIntern("ladder/ask_depletion")
	SymbolLadderBidReplenish   = nomagique.MustIntern("ladder/bid_replenish")
	SymbolLadderAskReplenish   = nomagique.MustIntern("ladder/ask_replenish")
	SymbolLadderSpreadBaseline = nomagique.MustIntern("ladder/spread_baseline")
	SymbolLadderMaturity       = nomagique.MustIntern("ladder/maturity")
	SymbolLadderReady          = nomagique.MustIntern("ladder/ready")

	SymbolLadderLastSec  = nomagique.MustIntern("ladder/last_sec")
	SymbolLadderLastNsec = nomagique.MustIntern("ladder/last_nsec")

	SymbolLadderSpreadBaselineState = nomagique.MustIntern("ladder/state/spread_baseline")
	SymbolLadderSpreadCountState    = nomagique.MustIntern("ladder/state/spread_count")
	SymbolLadderBidDepleteState     = nomagique.MustIntern("ladder/state/bid_deplete_baseline")
	SymbolLadderBidDepleteCount     = nomagique.MustIntern("ladder/state/bid_deplete_count")
	SymbolLadderAskDepleteState     = nomagique.MustIntern("ladder/state/ask_deplete_baseline")
	SymbolLadderAskDepleteCount     = nomagique.MustIntern("ladder/state/ask_deplete_count")
	SymbolLadderBidFillState        = nomagique.MustIntern("ladder/state/bid_fill_baseline")
	SymbolLadderBidFillCount        = nomagique.MustIntern("ladder/state/bid_fill_count")
	SymbolLadderAskFillState        = nomagique.MustIntern("ladder/state/ask_fill_baseline")
	SymbolLadderAskFillCount        = nomagique.MustIntern("ladder/state/ask_fill_count")
)

/*
Ladder composes the book-ladder conditioning transition. It receives one pass
of the committed resident book — resting depth per side, touch prices, and
event time — and derives signed depth changes from its own prior observation.

Every baseline is time-elastic: an exponentially decayed estimate whose
adaptation rate is set by the event-time gaps the symbol itself produces.
One physical halflife therefore means a dense book like BTC re-adapts
continuously while a quiet meme-coin holds its notion of normal between
sparse events — the retention budget is the data, not a count. The first
observation of anything seeds its baseline with itself, so the mean of one
value is that value.

The ladder judges nothing. Whether depth left the book because it was bought
or because it was pulled belongs to the honesty perspective; the ladder only
reports geometry and its dynamics.
*/
func Ladder() nomagique.Primitive {
	return nomagique.Pipe(ladderTransition)
}

func ladderTransition(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	observation, err := ladderObservation(state, input)

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	nextState := state
	elapsed, seeded, err := ladderElapsed(&nextState, observation)

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	nextState.Put(SymbolLadderBidDepth, observation.bidDepth)
	nextState.Put(SymbolLadderAskDepth, observation.askDepth)
	nextState.Put(SymbolLadderSpread, observation.spread)
	nextState.Put(SymbolLadderBidDelta, observation.bidDelta)
	nextState.Put(SymbolLadderAskDelta, observation.askDelta)
	nextState.Put(SymbolBid, observation.bid)
	nextState.Put(SymbolAsk, observation.ask)
	nextState.Put(SymbolMidpoint, observation.midpoint)
	nextState.Put(SymbolUnixSec, observation.sec)
	nextState.Put(SymbolUnixNsec, observation.nsec)

	alpha := 0.0

	if elapsed > 0 {
		tau := observation.halflife / math.Ln2
		alpha = 1 - math.Exp(-elapsed/tau)
	}

	ladderObserve(&nextState, SymbolLadderSpreadBaselineState,
		SymbolLadderSpreadCountState, alpha, seeded, observation.spread)

	if observation.bidDelta < 0 {
		ladderObserve(&nextState, SymbolLadderBidDepleteState,
			SymbolLadderBidDepleteCount, alpha, seeded, -observation.bidDelta)
	}

	if observation.askDelta < 0 {
		ladderObserve(&nextState, SymbolLadderAskDepleteState,
			SymbolLadderAskDepleteCount, alpha, seeded, -observation.askDelta)
	}

	if observation.bidDelta > 0 {
		ladderObserve(&nextState, SymbolLadderBidFillState,
			SymbolLadderBidFillCount, alpha, seeded, observation.bidDelta)
	}

	if observation.askDelta > 0 {
		ladderObserve(&nextState, SymbolLadderAskFillState,
			SymbolLadderAskFillCount, alpha, seeded, observation.askDelta)
	}

	output, err := composeLadder(nextState, observation)

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	return nextState, output, nil
}

/*
ladderObservation validates one pass of ladder inputs.
*/
func ladderObservation(
	state nomagique.Frame,
	input nomagique.Frame,
) (ladderInputs, error) {
	halflife, hasHalflife := input.Get(SymbolLadderHalflife)
	bidDepth, hasBid := input.Get(SymbolLadderBidDepth)
	askDepth, hasAsk := input.Get(SymbolLadderAskDepth)
	bid, hasBidPrice := input.Get(SymbolBid)
	ask, hasAskPrice := input.Get(SymbolAsk)
	sec, hasSec := input.Get(SymbolUnixSec)
	nsec, hasNsec := input.Get(SymbolUnixNsec)

	if !hasHalflife || !hasBid || !hasAsk || !hasBidPrice ||
		!hasAskPrice || !hasSec || !hasNsec {
		return ladderInputs{}, fmt.Errorf(
			"ladder: halflife, depths, touch prices, and event time are required",
		)
	}

	spread := ask - bid

	if halflife <= 0 || bid <= 0 || ask <= bid ||
		nsec < 0 || nsec >= 1e9 {
		return ladderInputs{}, fmt.Errorf(
			"ladder: halflife and touch must be positive and nanoseconds normalized",
		)
	}

	if bidDepth < 0 || askDepth < 0 ||
		!finite(bidDepth) || !finite(askDepth) || !finite(spread) ||
		!finite(bid) || !finite(ask) {
		return ladderInputs{}, fmt.Errorf(
			"ladder: depths and touch prices must be finite",
		)
	}

	previousBidDepth, hasPreviousBid := state.Get(SymbolLadderBidDepth)
	previousAskDepth, hasPreviousAsk := state.Get(SymbolLadderAskDepth)
	previousSpread, hasPreviousSpread := state.Get(SymbolLadderSpread)
	bidDelta := 0.0
	askDelta := 0.0

	if hasPreviousBid && hasPreviousAsk {
		bidDelta = bidDepth - previousBidDepth
		askDelta = askDepth - previousAskDepth
	}

	return ladderInputs{
		halflife: halflife,
		bidDepth: bidDepth,
		askDepth: askDepth,
		bid:      bid,
		ask:      ask,
		midpoint: bid + spread/2,
		previousSpread: previousSpread,
		hasPreviousSpread: hasPreviousSpread,
		spread:   spread,
		bidDelta: bidDelta,
		askDelta: askDelta,
		sec:      sec,
		nsec:     nsec,
	}, nil
}

type ladderInputs struct {
	halflife float64
	bidDepth float64
	askDepth float64
	bid      float64
	ask      float64
	midpoint float64
	previousSpread float64
	hasPreviousSpread bool
	spread   float64
	bidDelta float64
	askDelta float64
	sec      float64
	nsec     float64
}

/*
ladderElapsed advances the shared event clock and returns the elapsed seconds
since the previous pass. Event time must not regress.
*/
func ladderElapsed(
	state *nomagique.Frame,
	observation ladderInputs,
) (float64, bool, error) {
	previousSec, hasSec := state.Get(SymbolLadderLastSec)
	previousNsec, hasNsec := state.Get(SymbolLadderLastNsec)

	if !hasSec || !hasNsec {
		state.Put(SymbolLadderLastSec, observation.sec)
		state.Put(SymbolLadderLastNsec, observation.nsec)
		return 0, true, nil
	}

	elapsed := elapsed(observation.sec, observation.nsec, previousSec, previousNsec)

	if elapsed < 0 {
		return 0, false, fmt.Errorf(
			"ladder: event time must not regress",
		)
	}

	state.Put(SymbolLadderLastSec, observation.sec)
	state.Put(SymbolLadderLastNsec, observation.nsec)

	return elapsed, false, nil
}

/*
ladderObserve folds one magnitude into its time-elastic baseline. The first
observation seeds the baseline with itself; later ones decay toward the new
value at the alpha the event-time gap implies.
*/
func ladderObserve(
	state *nomagique.Frame,
	baselineSymbol nomagique.Symbol,
	countSymbol nomagique.Symbol,
	alpha float64,
	seeded bool,
	value float64,
) {
	baseline, hasBaseline := state.Get(baselineSymbol)

	if !hasBaseline || seeded {
		state.Put(baselineSymbol, value)
		state.Put(countSymbol, 1)
		return
	}

	count := number(*state, countSymbol)
	state.Put(baselineSymbol, (1-alpha)*baseline+alpha*value)
	state.Put(countSymbol, count+1)
}

/*
composeLadder derives the ladder metrics from retained baselines.
*/
func composeLadder(
	state nomagique.Frame,
	observation ladderInputs,
) (nomagique.Frame, error) {
	output := state
	output.Put(SymbolLadderBidDepth, observation.bidDepth)
	output.Put(SymbolLadderAskDepth, observation.askDepth)
	output.Put(SymbolLadderSpread, observation.spread)
	output.Put(SymbolLadderBidDepletion, 0)
	output.Put(SymbolLadderAskDepletion, 0)
	output.Put(SymbolLadderBidReplenish, 0)
	output.Put(SymbolLadderAskReplenish, 0)

	if observation.hasPreviousSpread && observation.previousSpread > 0 {
		output.Put(
			SymbolLadderSpreadTightening,
			math.Max(
				observation.previousSpread-observation.spread,
				0,
			)/observation.previousSpread,
		)
	}

	if observation.bidDepth > 0 && observation.askDepth > 0 {
		output.Put(SymbolLadderImbalance,
			math.Log(observation.bidDepth)-math.Log(observation.askDepth),
		)
	}

	spreadBaseline, spreadCount := ladderBaseline(state,
		SymbolLadderSpreadBaselineState, SymbolLadderSpreadCountState)

	if spreadCount > 0 {
		output.Put(SymbolLadderSpreadBaseline, spreadBaseline)

		if spreadBaseline > 0 {
			output.Put(
				SymbolLadderSpreadDeviation,
				(observation.spread-spreadBaseline)/spreadBaseline,
			)
			output.Put(SymbolLadderCompression,
				math.Max(0, 1-observation.spread/spreadBaseline),
			)
		}

		output.Put(SymbolLadderReady, 1)
	}

	scores := []struct {
		baseline nomagique.Symbol
		count    nomagique.Symbol
		output   nomagique.Symbol
		weight   float64
	}{
		{SymbolLadderBidDepleteState, SymbolLadderBidDepleteCount,
			SymbolLadderBidDepletion, -observation.bidDelta},
		{SymbolLadderAskDepleteState, SymbolLadderAskDepleteCount,
			SymbolLadderAskDepletion, -observation.askDelta},
		{SymbolLadderBidFillState, SymbolLadderBidFillCount,
			SymbolLadderBidReplenish, observation.bidDelta},
		{SymbolLadderAskFillState, SymbolLadderAskFillCount,
			SymbolLadderAskReplenish, observation.askDelta},
	}

	support := spreadCount

	for _, score := range scores {
		baseline, count := ladderBaseline(state, score.baseline, score.count)

		if count == 0 {
			continue
		}

		if score.weight > 0 && baseline > 0 {
			output.Put(score.output, score.weight/(baseline+score.weight))
		}

		if count < support || support == 0 {
			support = count
		}
	}

	output.Put(SymbolLadderMaturity, float64(support)/float64(support+1))

	return output, nil
}

func ladderBaseline(
	state nomagique.Frame,
	baselineSymbol nomagique.Symbol,
	countSymbol nomagique.Symbol,
) (float64, int) {
	baseline, hasBaseline := state.Get(baselineSymbol)

	if !hasBaseline {
		return 0, 0
	}

	return baseline, int(number(state, countSymbol))
}
