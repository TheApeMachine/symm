package types

/*
categoryPolarity is the directional prior of each semantic category: +1 reads as
"price up next", -1 as "price down next", 0 as no directional content. The fused
60-second price prediction uses polarity × confidence as each source's feature.

EDITABLE ASSUMPTION — this table is the one place a human judgment enters the
prediction loop. The online fusion weights learn how much (and whether) to trust
each source, so a wrong sign here is corrected by a negative learned weight, but
a sensible prior shortens the learning. Adjust freely; everything downstream
adapts.
*/
var categoryPolarity = map[CategoryType]float64{
	// pump/dump ignition
	CategoryVerticalIgnition:  +1.0,
	CategoryCoiledCompression: +0.5,
	CategoryOrganicTrend:      +0.5,
	CategoryFadedExhaustion:   -0.5,

	// exhaustion / reversal
	CategoryActiveReversal:     -0.5,
	CategoryThermalExhaustion:  -0.5,
	CategoryMechanicalCollapse: -1.0,

	// flow / volume
	CategoryAggressiveDrive:  +0.75,
	CategoryHiddenAbsorption: +0.5,
	CategoryVolumeStarvation: -0.25,

	// cross-market
	CategoryRiskOnSurge:       +0.75,
	CategorySystemicSlump:     -0.75,
	CategorySystemicBeta:      0,
	CategorySynchronizedDrift: +0.25,
	CategoryDecoupledMove:     0,
	CategoryDivergentMove:     0,

	// liquidity / book
	CategoryLiquidityVacuum: -0.5,
	CategoryBookThinning:    -0.5,
	CategoryExtremeScarcity: -0.25,
	CategoryRobustLiquidity: +0.25,
	CategoryHardSupport:     +0.5,
	CategoryLoadedImbalance: +0.5,
	CategoryDenseNeutrality: 0,

	// toxicity / stress
	CategoryToxicBluff:      -0.25,
	CategorySpoofTrap:       -0.25,
	CategoryTurbulent:       0,
	CategoryFrenzy:          0,
	CategoryLiquidityShock:  -0.5,
	CategorySystemicHerd:    0,
	CategoryDivergentStress: -0.25,

	// neutral states
	CategoryLaminar:           +0.25,
	CategoryStochasticNoise:   0,
	CategoryStochasticBalance: 0,
	CategoryAnchorStall:       0,
}

/*
CategoryPolarity returns the directional prior for a category (0 when unmapped).
*/
func CategoryPolarity(category CategoryType) float64 {
	return categoryPolarity[category]
}
