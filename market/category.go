package market

import (
	"fmt"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
CategoryFromEngine maps a signal's engine.Category string onto the uint8
CategoryType keys used in perspective trees (market/perspectives). Signals
classify in engine vocabulary; playbooks (pump, dump, …) branch on
perspectives.CategoryType.
*/
func CategoryFromEngine(category engine.Category) (perspectives.CategoryType, error) {
	switch category {
	case engine.CatLaminar:
		return perspectives.CategoryLaminar, nil
	case engine.CatTurbulent:
		return perspectives.CategoryTurbulent, nil
	case engine.CatInertial:
		return perspectives.CategoryInertial, nil
	case engine.CatViscous:
		return perspectives.CategoryViscous, nil
	case engine.CatFrenzy:
		return perspectives.CategoryFrenzy, nil
	case engine.CatSaturation:
		return perspectives.CategorySaturation, nil
	case engine.CatOrganic:
		return perspectives.CategoryOrganic, nil
	case engine.CatExhaustion:
		return perspectives.CategoryExhaustion, nil
	case engine.CatHiddenAbsorption:
		return perspectives.CategoryHiddenAbsorption, nil
	case engine.CatAggressiveDrive:
		return perspectives.CategoryAggressiveDrive, nil
	case engine.CatStochasticBalance:
		return perspectives.CategoryStochasticBalance, nil
	case engine.CatVolumeStarvation:
		return perspectives.CategoryVolumeStarvation, nil
	case engine.CatLoadedImbalance:
		return perspectives.CategoryLoadedImbalance, nil
	case engine.CatSpoofTrap:
		return perspectives.CategorySpoofTrap, nil
	case engine.CatBookThinning:
		return perspectives.CategoryBookThinning, nil
	case engine.CatDenseNeutrality:
		return perspectives.CategoryDenseNeutrality, nil
	case engine.CatInefficientLag:
		return perspectives.CategoryInefficientLag, nil
	case engine.CatSynchronizedDrift:
		return perspectives.CategorySynchronizedDrift, nil
	case engine.CatDecoupledMove:
		return perspectives.CategoryDecoupledMove, nil
	case engine.CatAnchorStall:
		return perspectives.CategoryAnchorStall, nil
	case engine.CatVerticalIgnition:
		return perspectives.CategoryVerticalIgnition, nil
	case engine.CatCoiledCompression:
		return perspectives.CategoryCoiledCompression, nil
	case engine.CatOrganicTrend:
		return perspectives.CategoryOrganicTrend, nil
	case engine.CatFadedExhaustion:
		return perspectives.CategoryFadedExhaustion, nil
	case engine.CatExtremeScarcity:
		return perspectives.CategoryExtremeScarcity, nil
	case engine.CatMedianDepth:
		return perspectives.CategoryMedianDepth, nil
	case engine.CatRobustLiquidity:
		return perspectives.CategoryRobustLiquidity, nil
	case engine.CatRiskOnSurge:
		return perspectives.CategoryRiskOnSurge, nil
	case engine.CatDivergentMove:
		return perspectives.CategoryDivergentMove, nil
	case engine.CatSystemicSlump:
		return perspectives.CategorySystemicSlump, nil
	case engine.CatLiquidityVacuum:
		return perspectives.CategoryLiquidityVacuum, nil
	case engine.CatToxicBluff:
		return perspectives.CategoryToxicBluff, nil
	case engine.CatHardSupport:
		return perspectives.CategoryHardSupport, nil
	case engine.CatSystemicHerd:
		return perspectives.CategorySystemicHerd, nil
	case engine.CatDecoupledAlpha:
		return perspectives.CategoryDecoupledAlpha, nil
	case engine.CatStochasticNoise:
		return perspectives.CategoryStochasticNoise, nil
	case engine.CatDivergentStress:
		return perspectives.CategoryDivergentStress, nil
	case engine.CatEndogenousAlpha:
		return perspectives.CategoryEndogenousAlpha, nil
	case engine.CatSystemicBeta:
		return perspectives.CategorySystemicBeta, nil
	case engine.CatLiquidityShock:
		return perspectives.CategoryLiquidityShock, nil
	case engine.CatCausalNoise:
		return perspectives.CategoryCausalNoise, nil
	case engine.CatMechanicalCollapse:
		return perspectives.CategoryMechanicalCollapse, nil
	case engine.CatThermalExhaustion:
		return perspectives.CategoryThermalExhaustion, nil
	case engine.CatFragileExpansion:
		return perspectives.CategoryFragileExpansion, nil
	case engine.CatActiveReversal:
		return perspectives.CategoryActiveReversal, nil
	case engine.CategoryNone:
		return perspectives.CategoryTypeNone, nil
	default:
		return perspectives.CategoryTypeNone, fmt.Errorf("market: unknown engine category %q", category)
	}
}
