package perspectives

type CategoryType uint8

const (
	CategoryTypeNone CategoryType = iota
	CategoryLaminar
	CategoryTurbulent
	CategoryInertial
	CategoryViscous
	CategoryFrenzy
	CategorySaturation
	CategoryOrganic
	CategoryExhaustion
	CategoryHiddenAbsorption
	CategoryAggressiveDrive
	CategoryStochasticBalance
	CategoryVolumeStarvation
	CategoryLoadedImbalance
	CategorySpoofTrap
	CategoryBookThinning
	CategoryDenseNeutrality
	CategoryInefficientLag
	CategorySynchronizedDrift
	CategoryDecoupledMove
	CategoryAnchorStall
	CategoryVerticalIgnition
	CategoryCoiledCompression
	CategoryOrganicTrend
	CategoryFadedExhaustion
	CategoryExtremeScarcity
	CategoryMedianDepth
	CategoryRobustLiquidity
	CategoryRiskOnSurge
	CategoryDivergentMove
	CategorySystemicSlump
	CategoryLiquidityVacuum
	CategoryToxicBluff
	CategoryHardSupport
	CategorySystemicHerd
	CategoryDecoupledAlpha
	CategoryStochasticNoise
	CategoryDivergentStress
	CategoryEndogenousAlpha
	CategorySystemicBeta
	CategoryLiquidityShock
	CategoryCausalNoise
	CategoryMechanicalCollapse
	CategoryThermalExhaustion
	CategoryFragileExpansion
	CategoryActiveReversal
)

// categoryNames is the human-readable label for each category, used in audit
// trails and the dashboard.
var categoryNames = map[CategoryType]string{
	CategoryLaminar:            "laminar",
	CategoryTurbulent:          "turbulent",
	CategoryInertial:           "inertial",
	CategoryViscous:            "viscous",
	CategoryFrenzy:             "frenzy",
	CategorySaturation:         "saturation",
	CategoryOrganic:            "organic",
	CategoryExhaustion:         "exhaustion",
	CategoryHiddenAbsorption:   "hidden_absorption",
	CategoryAggressiveDrive:    "aggressive_drive",
	CategoryStochasticBalance:  "stochastic_balance",
	CategoryVolumeStarvation:   "volume_starvation",
	CategoryLoadedImbalance:    "loaded_imbalance",
	CategorySpoofTrap:          "spoof_trap",
	CategoryBookThinning:       "book_thinning",
	CategoryDenseNeutrality:    "dense_neutrality",
	CategoryInefficientLag:     "inefficient_lag",
	CategorySynchronizedDrift:  "synchronized_drift",
	CategoryDecoupledMove:      "decoupled_move",
	CategoryAnchorStall:        "anchor_stall",
	CategoryVerticalIgnition:   "vertical_ignition",
	CategoryCoiledCompression:  "coiled_compression",
	CategoryOrganicTrend:       "organic_trend",
	CategoryFadedExhaustion:    "faded_exhaustion",
	CategoryExtremeScarcity:    "extreme_scarcity",
	CategoryMedianDepth:        "median_depth",
	CategoryRobustLiquidity:    "robust_liquidity",
	CategoryRiskOnSurge:        "risk_on_surge",
	CategoryDivergentMove:      "divergent_move",
	CategorySystemicSlump:      "systemic_slump",
	CategoryLiquidityVacuum:    "liquidity_vacuum",
	CategoryToxicBluff:         "toxic_bluff",
	CategoryHardSupport:        "hard_support",
	CategorySystemicHerd:       "systemic_herd",
	CategoryDecoupledAlpha:     "decoupled_alpha",
	CategoryStochasticNoise:    "stochastic_noise",
	CategoryDivergentStress:    "divergent_stress",
	CategoryEndogenousAlpha:    "endogenous_alpha",
	CategorySystemicBeta:       "systemic_beta",
	CategoryLiquidityShock:     "liquidity_shock",
	CategoryCausalNoise:        "causal_noise",
	CategoryMechanicalCollapse: "mechanical_collapse",
	CategoryThermalExhaustion:  "thermal_exhaustion",
	CategoryFragileExpansion:   "fragile_expansion",
	CategoryActiveReversal:     "active_reversal",
}

/*
String returns the category's dashboard label (empty for CategoryTypeNone).
*/
func (category CategoryType) String() string {
	return categoryNames[category]
}
