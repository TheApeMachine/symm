package correlation

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolCohortCorrelation    = types.MustIntern("correlation/cohort")
	SymbolSignedCorrelation    = types.MustIntern("correlation/signed")
	SymbolRelativeEnergy       = types.MustIntern("correlation/relative_energy")
	SymbolHerd                 = types.MustIntern("correlation/herd")
	SymbolAlpha                = types.MustIntern("correlation/alpha")
	SymbolNoise                = types.MustIntern("correlation/noise")
	SymbolStress               = types.MustIntern("correlation/stress")
	SymbolHypothesisSeparation = types.MustIntern("correlation/hypothesis_separation")
)

/*
Scores projects support-weighted pair statistics into four competing
hypotheses: coherent herd motion, focal excess energy, incoherent noise, and
opposing stress. Every score is normalized by observed correlation or energy.

It is a primitive: the accumulated cohort statistics arrive in state (produced
by the Cohort reducer), and the four hypothesis scores plus their separation
leave in the output.
*/
func Scores(
	state types.Frame,
	input types.Frame,
) (types.Frame, types.Frame, error) {
	ready, found := state.Get(SymbolReady)
	totalSupport, hasSupport := state.Get(SymbolTotalSupport)
	peerEnergyTotal, hasPeerEnergy := state.Get(SymbolWeightedPeerEnergy)
	focalEnergy, hasFocalEnergy := state.Get(SymbolFocalEnergy)

	if !found || ready == 0 || !hasSupport || totalSupport <= 0 ||
		!hasPeerEnergy || !hasFocalEnergy {
		output := input
		output.Put(SymbolReady, 0)

		return state, output, nil
	}

	weightedSigned := state.MustGet(SymbolWeightedSigned)
	weightedAbsolute := state.MustGet(SymbolWeightedAbsolute)
	peerEnergy := peerEnergyTotal / totalSupport
	signed := weightedSigned / totalSupport
	cohort := weightedAbsolute / totalSupport
	relativeEnergy := focalEnergy / peerEnergy
	energyScale := focalEnergy + peerEnergy
	alpha := math.Max(0, focalEnergy-peerEnergy) / energyScale
	herd := math.Max(0, signed) * math.Min(1, peerEnergy/focalEnergy)
	noise := math.Max(0, 1-cohort)
	stress := math.Max(0, -signed)
	separation := hypothesisSeparation(herd, alpha, noise, stress)

	output := input
	output.Put(SymbolCohortCorrelation, cohort)
	output.Put(SymbolSignedCorrelation, signed)
	output.Put(SymbolRelativeEnergy, relativeEnergy)
	output.Put(SymbolHerd, herd)
	output.Put(SymbolAlpha, alpha)
	output.Put(SymbolNoise, noise)
	output.Put(SymbolStress, stress)
	output.Put(SymbolHypothesisSeparation, separation)
	output.Put(SymbolReady, 1)

	return state, output, nil
}

func hypothesisSeparation(scores ...float64) float64 {
	for index := 1; index < len(scores); index++ {
		value := scores[index]
		position := index

		for position > 0 && scores[position-1] < value {
			scores[position] = scores[position-1]
			position--
		}

		scores[position] = value
	}

	if len(scores) < 2 || scores[0] <= 0 {
		return 0
	}

	return (scores[0] - scores[1]) / scores[0]
}
