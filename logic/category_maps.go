package logic

/*
CategoryIndexMaps holds 1-based transition indices for each signal source.
Index 0 is reserved as the none/rest state in transition matrices.
*/
var CategoryIndexMaps = map[SourceType]map[CategoryType]int{
	SourceCVD: {
		CategoryHiddenAbsorption:  1,
		CategoryAggressiveDrive:   2,
		CategoryStochasticBalance: 3,
		CategoryVolumeStarvation:  4,
	},
	SourceCausal: {
		CategoryEndogenousAlpha: 1,
		CategoryLiquidityShock:  2,
		CategorySystemicBeta:    3,
		CategoryCausalNoise:     4,
	},
	SourceCorrelation: {
		CategorySystemicHerd:    1,
		CategoryDecoupledAlpha:  2,
		CategoryStochasticNoise: 3,
		CategoryDivergentStress: 4,
	},
	SourceDepthFlow: {
		CategoryLoadedImbalance: 1,
		CategorySpoofTrap:       2,
		CategoryBookThinning:    3,
		CategoryDenseNeutrality: 4,
	},
	SourceExhaustion: {
		CategoryMechanicalCollapse: 1,
		CategoryFragileExpansion:   2,
		CategoryThermalExhaustion:  3,
		CategoryActiveReversal:     4,
	},
	SourceFluid: {
		CategoryLaminar:   1,
		CategoryTurbulent: 2,
		CategoryInertial:  3,
		CategoryViscous:   4,
	},
	SourceHawkes: {
		CategoryFrenzy:     1,
		CategorySaturation: 2,
		CategoryOrganic:    3,
		CategoryExhaustion: 4,
	},
	SourceLeadLag: {
		CategoryInefficientLag:    1,
		CategorySynchronizedDrift: 2,
		CategoryDecoupledMove:     3,
		CategoryAnchorStall:       4,
	},
	SourceLiquidity: {
		CategoryExtremeScarcity: 1,
		CategoryMedianDepth:     2,
		CategoryRobustLiquidity: 3,
	},
	SourceManifold: {
		CategorySystemicHerd:      1,
		CategoryLiquidityShock:    2,
		CategorySynchronizedDrift: 3,
		CategoryStochasticNoise:   4,
	},
	SourcePumpDump: {
		CategoryVerticalIgnition:  1,
		CategoryCoiledCompression: 2,
		CategoryOrganicTrend:      3,
		CategoryFadedExhaustion:   4,
	},
	SourceSentiment: {
		CategoryRiskOnSurge:   1,
		CategoryDivergentMove: 2,
		CategorySystemicSlump: 3,
	},
	SourceToxicity: {
		CategoryToxicBluff:      1,
		CategoryLiquidityVacuum: 2,
		CategoryHardSupport:     3,
	},
}

/*
CategoryIndexFor returns the 1-based transition index for a source/category pair.
*/
func CategoryIndexFor(source SourceType, category CategoryType) int {
	mapping, ok := CategoryIndexMaps[source]

	if !ok {
		return CategoryNoneIndex
	}

	return RealCategoryIndex(category, mapping)
}
