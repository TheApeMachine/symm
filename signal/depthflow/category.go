package depthflow

import (
	"math"

	"github.com/theapemachine/symm/engine"
)

/*
depthflowCategory maps book-shape reads onto the weight-of-the-book perspective.
*/
func depthflowCategory(
	reason string,
	weightedImbalance float64,
	flatImbalance float64,
	flatOK bool,
) engine.Category {
	if reason == "depth_skeptic" {
		return engine.CatSpoofTrap
	}

	if reason == "book_thinning" {
		return engine.CatBookThinning
	}

	if reason == "depth_imbalance" {
		return engine.CatLoadedImbalance
	}

	if flatOK && math.Abs(weightedImbalance) > 0 &&
		math.Abs(flatImbalance) < math.Abs(weightedImbalance)*0.5 {
		return engine.CatBookThinning
	}

	return engine.CatDenseNeutrality
}
