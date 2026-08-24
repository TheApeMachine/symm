package correlation

import (
	"math"

	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolTotalSupport       = nmtypes.MustIntern("correlation/cohort/support")
	SymbolWeightedSigned     = nmtypes.MustIntern("correlation/cohort/weighted_signed")
	SymbolWeightedAbsolute   = nmtypes.MustIntern("correlation/cohort/weighted_absolute")
	SymbolWeightedPeerEnergy = nmtypes.MustIntern("correlation/cohort/weighted_peer_energy")
	SymbolFocalEnergy        = nmtypes.MustIntern("correlation/cohort/focal_energy")
	SymbolPeerCount          = nmtypes.MustIntern("correlation/cohort/peer_count")
	SymbolCohortDispersion   = nmtypes.MustIntern("correlation/cohort/dispersion")
	SymbolEffectivePeers     = nmtypes.MustIntern("correlation/cohort/effective_peers")
	SymbolWeightedFisherZ    = nmtypes.MustIntern("correlation/cohort/weighted_fisher_z")
	SymbolWeightedFisherZ2   = nmtypes.MustIntern("correlation/cohort/weighted_fisher_z2")
	SymbolWeightSquares      = nmtypes.MustIntern("correlation/cohort/weight_squares")
)

const minimumCorrelationSupport = 2

/*
Cohort accumulates the sufficient statistics emitted by ready Hayashi pairs.
Support weights each peer so asynchronous paths contribute in proportion to
the return intervals that actually overlapped.
*/
func Cohort(input nmtypes.Frame) nmtypes.Frame {
	ready, hasReady := input.Get(SymbolReady)
	support, hasSupport := input.Get(SymbolSupport)

	if !hasReady || ready == 0 || !hasSupport || support < minimumCorrelationSupport {
		input.Put(SymbolReady, 0)

		return input
	}

	correlation := input.MustGet(SymbolCorrelation)
	leftVariance := input.MustGet(SymbolLeftVariance)
	rightVariance := input.MustGet(SymbolRightVariance)
	totalSupport, _ := input.Get(SymbolTotalSupport)
	weightedSigned, _ := input.Get(SymbolWeightedSigned)
	weightedAbsolute, _ := input.Get(SymbolWeightedAbsolute)
	weightedPeerEnergy, _ := input.Get(SymbolWeightedPeerEnergy)
	weightedFisherZ, _ := input.Get(SymbolWeightedFisherZ)
	weightedFisherZ2, _ := input.Get(SymbolWeightedFisherZ2)
	weightSquares, _ := input.Get(SymbolWeightSquares)
	peerCount, _ := input.Get(SymbolPeerCount)

	fisherZ := math.Atanh(clampCorrelation(correlation))
	totalSupport += support
	weightedSigned += correlation * support
	weightedAbsolute += absolute(correlation) * support
	weightedPeerEnergy += rightVariance * support
	weightedFisherZ += fisherZ * support
	weightedFisherZ2 += fisherZ * fisherZ * support
	weightSquares += support * support
	peerCount++

	meanZ := weightedFisherZ / totalSupport
	dispersion := math.Sqrt(
		weightedFisherZ2/totalSupport - meanZ*meanZ,
	)
	effectivePeers := 0.0

	if weightSquares > 0 {
		effectivePeers = totalSupport * totalSupport / weightSquares
	}

	input.Put(SymbolTotalSupport, totalSupport)
	input.Put(SymbolWeightedSigned, weightedSigned)
	input.Put(SymbolWeightedAbsolute, weightedAbsolute)
	input.Put(SymbolWeightedPeerEnergy, weightedPeerEnergy)
	input.Put(SymbolFocalEnergy, leftVariance)
	input.Put(SymbolPeerCount, peerCount)
	input.Put(SymbolCohortDispersion, dispersion)
	input.Put(SymbolEffectivePeers, effectivePeers)
	input.Put(SymbolReady, 1)

	return input
}

func clampCorrelation(value float64) float64 {
	return math.Max(-1, math.Min(1, value))
}

func absolute(value float64) float64 {
	if value < 0 {
		return -value
	}

	return value
}
