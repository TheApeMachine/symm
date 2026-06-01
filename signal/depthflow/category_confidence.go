package depthflow

import (
	"math"

	"github.com/theapemachine/symm/market/perspectives"
)

const bookThinningFlatFraction = 0.5

/*
categoryConfidence returns how decisively the depth-flow category wins over its
neighbors — not how large the fused strength is.
*/
func categoryConfidence(
	category perspectives.CategoryType,
	weightedImbalance, flatImbalance float64,
	flatOK bool,
	flow float64,
) float64 {
	switch category {
	case perspectives.CategoryLoadedImbalance:
		return loadedImbalanceConfidence(weightedImbalance, flatImbalance, flatOK)
	case perspectives.CategoryBookThinning:
		return bookThinningConfidence(weightedImbalance, flatImbalance, flatOK)
	case perspectives.CategorySpoofTrap:
		return spoofTrapConfidence(weightedImbalance)
	case perspectives.CategoryDenseNeutrality:
		return denseNeutralityConfidence(weightedImbalance, flow)
	default:
		return 0
	}
}

func loadedImbalanceConfidence(weightedImbalance, flatImbalance float64, flatOK bool) float64 {
	if !flatOK {
		return math.Min(1, math.Abs(weightedImbalance))
	}

	boundary := math.Abs(weightedImbalance) * bookThinningFlatFraction
	margin := boundary - math.Abs(flatImbalance)

	if margin <= 0 {
		return 0
	}

	return margin / math.Max(boundary, 1e-12)
}

func bookThinningConfidence(weightedImbalance, flatImbalance float64, flatOK bool) float64 {
	if !flatOK {
		return 0
	}

	margin := math.Abs(flatImbalance) - math.Abs(weightedImbalance)*bookThinningFlatFraction

	if margin <= 0 {
		return 0
	}

	return margin / math.Max(math.Abs(flatImbalance), 1e-12)
}

func spoofTrapConfidence(weightedImbalance float64) float64 {
	return math.Min(1, math.Abs(weightedImbalance))
}

func denseNeutralityConfidence(weightedImbalance, flow float64) float64 {
	if flow > 0 {
		return math.Min(1, flow)
	}

	return math.Max(0, 1-math.Abs(weightedImbalance))
}
