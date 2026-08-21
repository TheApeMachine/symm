package correlation

import "github.com/theapemachine/symm/nomagique"

var (
	SymbolTotalSupport       = nomagique.MustIntern("correlation/cohort/support")
	SymbolWeightedSigned     = nomagique.MustIntern("correlation/cohort/weighted_signed")
	SymbolWeightedAbsolute   = nomagique.MustIntern("correlation/cohort/weighted_absolute")
	SymbolWeightedPeerEnergy = nomagique.MustIntern("correlation/cohort/weighted_peer_energy")
	SymbolFocalEnergy        = nomagique.MustIntern("correlation/cohort/focal_energy")
	SymbolPeerCount          = nomagique.MustIntern("correlation/cohort/peer_count")
)

const minimumCorrelationSupport = 2

/*
Cohort accumulates the sufficient statistics emitted by ready Hayashi pairs.
Support weights each peer so asynchronous paths contribute in proportion to
the return intervals that actually overlapped.
*/
func Cohort(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	ready, hasReady := input.Get(SymbolReady)
	support, hasSupport := input.Get(SymbolSupport)

	if !hasReady || ready == 0 || !hasSupport || support < minimumCorrelationSupport {
		output := state
		output.Put(SymbolReady, 0)

		return state, output, nil
	}

	correlation := input.MustGet(SymbolCorrelation)
	leftVariance := input.MustGet(SymbolLeftVariance)
	rightVariance := input.MustGet(SymbolRightVariance)
	totalSupport, _ := state.Get(SymbolTotalSupport)
	weightedSigned, _ := state.Get(SymbolWeightedSigned)
	weightedAbsolute, _ := state.Get(SymbolWeightedAbsolute)
	weightedPeerEnergy, _ := state.Get(SymbolWeightedPeerEnergy)
	peerCount, _ := state.Get(SymbolPeerCount)

	totalSupport += support
	weightedSigned += correlation * support
	weightedAbsolute += absolute(correlation) * support
	weightedPeerEnergy += rightVariance * support
	peerCount++

	nextState := state
	nextState.Put(SymbolTotalSupport, totalSupport)
	nextState.Put(SymbolWeightedSigned, weightedSigned)
	nextState.Put(SymbolWeightedAbsolute, weightedAbsolute)
	nextState.Put(SymbolWeightedPeerEnergy, weightedPeerEnergy)
	nextState.Put(SymbolFocalEnergy, leftVariance)
	nextState.Put(SymbolPeerCount, peerCount)
	nextState.Put(SymbolReady, 1)

	return nextState, nextState, nil
}

func absolute(value float64) float64 {
	if value < 0 {
		return -value
	}

	return value
}
