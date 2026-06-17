package logic

/*
CategoryFromSignalName maps a dashboard signal name and 1-based classifier index
to the canonical category token used in cognitive sequences.
*/
func CategoryFromSignalName(signalName string, categoryIndex int) CategoryType {
	if categoryIndex <= 0 {
		return CategoryTypeNone
	}

	switch signalName {
	case "causal":
		return causalCategory(categoryIndex)
	case "correlation":
		return correlationCategory(categoryIndex)
	case "cvd":
		return cvdCategory(categoryIndex)
	case "depthflow":
		return depthflowCategory(categoryIndex)
	case "exhaust":
		return exhaustCategory(categoryIndex)
	case "fluid":
		return fluidCategory(categoryIndex)
	case "hawkes":
		return hawkesCategory(categoryIndex)
	case "leadlag":
		return leadlagCategory(categoryIndex)
	case "liquidity":
		return liquidityCategory(categoryIndex)
	case "manifold":
		return manifoldCategory(categoryIndex)
	case "pumpdump":
		return pumpdumpCategory(categoryIndex)
	case "sentiment":
		return sentimentCategory(categoryIndex)
	case "toxicity":
		return toxicityCategory(categoryIndex)
	default:
		return CategoryTypeNone
	}
}

func causalCategory(categoryIndex int) CategoryType {
	switch categoryIndex {
	case 1:
		return CategoryEndogenousAlpha
	case 2:
		return CategoryLiquidityShock
	case 3:
		return CategorySystemicBeta
	case 4:
		return CategoryCausalNoise
	default:
		return CategoryTypeNone
	}
}

func correlationCategory(categoryIndex int) CategoryType {
	switch categoryIndex {
	case 1:
		return CategorySystemicHerd
	case 2:
		return CategoryDecoupledAlpha
	case 3:
		return CategoryStochasticNoise
	case 4:
		return CategoryDivergentStress
	default:
		return CategoryTypeNone
	}
}

func cvdCategory(categoryIndex int) CategoryType {
	switch categoryIndex {
	case 1:
		return CategoryHiddenAbsorption
	case 2:
		return CategoryAggressiveDrive
	case 3:
		return CategoryStochasticBalance
	case 4:
		return CategoryVolumeStarvation
	default:
		return CategoryTypeNone
	}
}

func depthflowCategory(categoryIndex int) CategoryType {
	switch categoryIndex {
	case 1:
		return CategoryLoadedImbalance
	case 2:
		return CategorySpoofTrap
	case 3:
		return CategoryBookThinning
	case 4:
		return CategoryDenseNeutrality
	default:
		return CategoryTypeNone
	}
}

func exhaustCategory(categoryIndex int) CategoryType {
	switch categoryIndex {
	case 1:
		return CategoryMechanicalCollapse
	case 2:
		return CategoryFragileExpansion
	case 3:
		return CategoryThermalExhaustion
	case 4:
		return CategoryActiveReversal
	default:
		return CategoryTypeNone
	}
}

func fluidCategory(categoryIndex int) CategoryType {
	switch categoryIndex {
	case 1:
		return CategoryLaminar
	case 2:
		return CategoryTurbulent
	case 3:
		return CategoryInertial
	case 4:
		return CategoryViscous
	default:
		return CategoryTypeNone
	}
}

func hawkesCategory(categoryIndex int) CategoryType {
	switch categoryIndex {
	case 1:
		return CategoryFrenzy
	case 2:
		return CategorySaturation
	case 3:
		return CategoryOrganic
	case 4:
		return CategoryExhaustion
	default:
		return CategoryTypeNone
	}
}

func leadlagCategory(categoryIndex int) CategoryType {
	switch categoryIndex {
	case 1:
		return CategoryInefficientLag
	case 2:
		return CategorySynchronizedDrift
	case 3:
		return CategoryDecoupledMove
	case 4:
		return CategoryAnchorStall
	default:
		return CategoryTypeNone
	}
}

func liquidityCategory(categoryIndex int) CategoryType {
	switch categoryIndex {
	case 1:
		return CategoryExtremeScarcity
	case 2:
		return CategoryMedianDepth
	case 3:
		return CategoryRobustLiquidity
	default:
		return CategoryTypeNone
	}
}

func manifoldCategory(categoryIndex int) CategoryType {
	switch categoryIndex {
	case 1:
		return CategorySystemicHerd
	case 2:
		return CategoryLiquidityShock
	case 3:
		return CategorySynchronizedDrift
	case 4:
		return CategoryStochasticNoise
	default:
		return CategoryTypeNone
	}
}

func pumpdumpCategory(categoryIndex int) CategoryType {
	switch categoryIndex {
	case 1:
		return CategoryVerticalIgnition
	case 2:
		return CategoryCoiledCompression
	case 3:
		return CategoryOrganicTrend
	case 4:
		return CategoryFadedExhaustion
	default:
		return CategoryTypeNone
	}
}

func sentimentCategory(categoryIndex int) CategoryType {
	switch categoryIndex {
	case 1:
		return CategoryRiskOnSurge
	case 2:
		return CategoryDivergentMove
	case 3:
		return CategorySystemicSlump
	default:
		return CategoryTypeNone
	}
}

func toxicityCategory(categoryIndex int) CategoryType {
	switch categoryIndex {
	case 1:
		return CategoryToxicBluff
	case 2:
		return CategoryLiquidityVacuum
	case 3:
		return CategoryHardSupport
	default:
		return CategoryTypeNone
	}
}
