package logic

import (
	"fmt"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

type categoryRole struct {
	direction float64
	risk      float64
	support   float64
}

func roleForCategory(category types.CategoryType) (categoryRole, error) {
	switch category {
	case types.CategoryLaminar,
		types.CategoryInertial,
		types.CategoryFrenzy,
		types.CategoryHiddenAbsorption,
		types.CategoryAggressiveDrive,
		types.CategoryLoadedImbalance,
		types.CategoryInefficientLag,
		types.CategoryVerticalIgnition,
		types.CategoryCoiledCompression,
		types.CategoryOrganicTrend,
		types.CategoryRobustLiquidity,
		types.CategoryRiskOnSurge,
		types.CategoryDecoupledAlpha,
		types.CategoryEndogenousAlpha,
		types.CategoryHardSupport:
		return categoryRole{direction: 1, support: 1}, nil
	case types.CategoryTurbulent,
		types.CategoryViscous,
		types.CategorySaturation,
		types.CategoryExhaustion,
		types.CategoryVolumeStarvation,
		types.CategorySpoofTrap,
		types.CategoryBookThinning,
		types.CategoryAnchorStall,
		types.CategoryFadedExhaustion,
		types.CategorySystemicSlump,
		types.CategoryLiquidityVacuum,
		types.CategoryToxicBluff,
		types.CategoryDivergentStress,
		types.CategoryLiquidityShock,
		types.CategoryMechanicalCollapse,
		types.CategoryThermalExhaustion,
		types.CategoryFragileExpansion,
		types.CategoryActiveReversal:
		return categoryRole{direction: -1, risk: 1}, nil
	case types.CategoryOrganic,
		types.CategoryStochasticBalance,
		types.CategoryDenseNeutrality,
		types.CategorySynchronizedDrift,
		types.CategoryDecoupledMove,
		types.CategoryExtremeScarcity,
		types.CategoryMedianDepth,
		types.CategoryDivergentMove,
		types.CategorySystemicHerd,
		types.CategoryStochasticNoise,
		types.CategorySystemicBeta,
		types.CategoryCausalNoise,
		types.CategoryLaminarResonance,
		types.CategoryTurbulentResonance,
		types.CategoryEquilibrium,
		types.CategoryForecastEdge:
		return categoryRole{}, nil
	}

	return categoryRole{}, errnie.Error(errnie.Err(
		errnie.Validation,
		fmt.Sprintf("decision boundary: category %q role required", category),
		nil,
	))
}
