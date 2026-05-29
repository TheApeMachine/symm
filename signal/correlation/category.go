package correlation

import (
	"math"

	"github.com/theapemachine/symm/engine"
)

const (
	systemicHerdCorrelation    = 0.85
	decoupledVarianceFloor     = 1e-8
	stochasticVarianceCeiling  = 1e-6
	divergentStressCorrelation = -0.3
)

/*
correlationCategory maps peak pairwise correlation and return variance onto the herd
behaviour perspective.
*/
func correlationCategory(correlation, variance float64) engine.Category {
	if correlation <= divergentStressCorrelation {
		return engine.CatDivergentStress
	}

	if correlation >= systemicHerdCorrelation {
		return engine.CatSystemicHerd
	}

	if math.Abs(correlation) < systemicHerdCorrelation {
		if variance >= decoupledVarianceFloor && variance > stochasticVarianceCeiling {
			return engine.CatDecoupledAlpha
		}

		return engine.CatStochasticNoise
	}

	return engine.CatSystemicHerd
}
