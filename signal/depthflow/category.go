package depthflow

import (
	"math"

	"github.com/theapemachine/symm/market/perspectives"
)

const (
	reasonDepthSkeptic  = "depth_skeptic"
	reasonBookThinning  = "book_thinning"
	reasonDepthImbalance = "depth_imbalance"
)

/*
depthflowCategory maps book-shape reads onto the weight-of-the-book perspective.
*/
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
		math.Abs(flatImbalance) < math.Abs(weightedImbalance)*0.5 {
		return perspectives.CategoryBookThinning
	}

	return perspectives.CategoryDenseNeutrality
}
