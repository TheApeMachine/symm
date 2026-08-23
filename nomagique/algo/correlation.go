package algo

import (
	"math"

	"github.com/theapemachine/symm/nomagique"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolCohortCorrelation    = nomagique.MustIntern("correlation/cohort")
	SymbolSignedCorrelation    = nomagique.MustIntern("correlation/signed")
	SymbolRelativeEnergy       = nomagique.MustIntern("correlation/relative_energy")
	SymbolHerd                 = nomagique.MustIntern("correlation/herd")
	SymbolAlpha                = nomagique.MustIntern("correlation/alpha")
	SymbolNoise                = nomagique.MustIntern("correlation/noise")
	SymbolStress               = nomagique.MustIntern("correlation/stress")
	SymbolHypothesisSeparation = nomagique.MustIntern("correlation/hypothesis_separation")
)

/*
Correlation projects support-weighted pair statistics into four competing
hypotheses: coherent herd motion, focal excess energy, incoherent noise, and
opposing stress. Every score is normalized by observed correlation or energy.
*/
func Correlation() nomagique.Primitive {
	return correlationScores
}

func correlationScores(
	state types.Frame,
	input types.Frame,
) (types.Frame, types.Frame, error) {
	ready, found := state.Get(nmcorrelation.SymbolReady)
	totalSupport, hasSupport := state.Get(nmcorrelation.SymbolTotalSupport)
	peerEnergyTotal, hasPeerEnergy := state.Get(nmcorrelation.SymbolWeightedPeerEnergy)
	focalEnergy, hasFocalEnergy := state.Get(nmcorrelation.SymbolFocalEnergy)

	if !found || ready == 0 || !hasSupport || totalSupport <= 0 ||
		!hasPeerEnergy || !hasFocalEnergy {
		output := input
		output.Put(nmcorrelation.SymbolReady, 0)

		return state, output, nil
	}

	weightedSigned := state.MustGet(nmcorrelation.SymbolWeightedSigned)
	weightedAbsolute := state.MustGet(nmcorrelation.SymbolWeightedAbsolute)
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
	output.Put(nmcorrelation.SymbolReady, 1)

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
