package depthflow

import (
	"math"

	"github.com/theapemachine/symm/market/perspectives"
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
) (perspectives.CategoryType, float64) {
	category := depthflowCategory(reason, weightedImbalance, flatImbalance, flatOK)

	switch category {
	case perspectives.CategoryLoadedImbalance:
		if !flatOK {
			return category, math.Min(1, math.Abs(weightedImbalance))
		}

		boundary := math.Abs(weightedImbalance) * bookThinningFlatFraction
		margin := boundary - math.Abs(flatImbalance)

		if margin <= 0 {
			return category, 0
		}

		return category, margin / math.Max(boundary, 1e-12)
	case perspectives.CategoryBookThinning:
		if !flatOK {
			return category, 0
		}

		margin := math.Abs(flatImbalance) - math.Abs(weightedImbalance)*bookThinningFlatFraction

		if margin <= 0 {
			return category, 0
		}

		return category, margin / math.Max(math.Abs(flatImbalance), 1e-12)
	case perspectives.CategorySpoofTrap:
		return category, math.Min(1, math.Abs(weightedImbalance))
	default:
		if flow > 0 {
			return category, math.Min(1, flow)
		}

		return category, math.Max(0, 1-math.Abs(weightedImbalance))
	}
}

func depthflowCategory(
	reason string,
	weightedImbalance float64,
	flatImbalance float64,
	flatOK bool,
) perspectives.CategoryType {
	if reason == reasonDepthSkeptic {
		return perspectives.CategorySpoofTrap
	}

	if reason == reasonBookThinning {
		return perspectives.CategoryBookThinning
	}

	if reason == reasonDepthImbalance {
		return perspectives.CategoryLoadedImbalance
	}

	if flatOK && math.Abs(weightedImbalance) > 0 &&
		math.Abs(flatImbalance) < math.Abs(weightedImbalance)*bookThinningFlatFraction {
		return perspectives.CategoryBookThinning
	}

	return perspectives.CategoryDenseNeutrality
}
