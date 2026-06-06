package depthflow

import (
	"math"

	"github.com/theapemachine/symm/market/perspectives/types"
)

const (
	reasonDepthSkeptic       = "depth_skeptic"
	reasonBookThinning       = "book_thinning"
	reasonDepthImbalance     = "depth_imbalance"
	bookThinningFlatFraction = 0.5
)

/*
depthflowReading classifies book shape and returns shift evidence.
*/
func depthflowReading(
	reason string,
	weightedImbalance, flatImbalance float64,
	flatOK bool,
	flow float64,
) (types.CategoryType, float64) {
	category := depthflowCategory(reason, weightedImbalance, flatImbalance, flatOK)

	switch category {
	case types.CategoryLoadedImbalance:
		if !flatOK {
			return category, types.UnitMagnitudeMargin(math.Abs(weightedImbalance))
		}

		boundary := math.Abs(weightedImbalance) * bookThinningFlatFraction
		margin := boundary - math.Abs(flatImbalance)

		if margin <= 0 {
			return category, types.UnitMarginFloor
		}

		return category, types.UnitCompetitionMargin(margin, boundary)
	case types.CategoryBookThinning:
		if !flatOK {
			return category, types.UnitMarginFloor
		}

		margin := math.Abs(flatImbalance) - math.Abs(weightedImbalance)*bookThinningFlatFraction

		if margin <= 0 {
			return category, types.UnitMarginFloor
		}

		return category, types.UnitCompetitionMargin(margin, math.Abs(flatImbalance))
	case types.CategorySpoofTrap:
		return category, types.UnitMagnitudeMargin(math.Abs(weightedImbalance))
	default:
		if flow > 0 {
			return category, types.UnitCompetitionMargin(flow, 1)
		}

		margin := 1 - math.Abs(weightedImbalance)

		if margin <= 0 {
			return category, types.UnitMarginFloor
		}

		return category, types.UnitCompetitionMargin(margin, 1)
	}
}

func depthflowCategory(
	reason string,
	weightedImbalance float64,
	flatImbalance float64,
	flatOK bool,
) types.CategoryType {
	if reason == reasonDepthSkeptic {
		return types.CategorySpoofTrap
	}

	if reason == reasonBookThinning {
		return types.CategoryBookThinning
	}

	if flatOK && math.Abs(weightedImbalance) > 0 &&
		math.Abs(flatImbalance) < math.Abs(weightedImbalance)*bookThinningFlatFraction {
		return types.CategoryBookThinning
	}

	if reason == reasonDepthImbalance {
		return types.CategoryLoadedImbalance
	}

	return types.CategoryDenseNeutrality
}
