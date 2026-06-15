package logic

type CategoryType string

const (
	CategoryTypeNone           CategoryType = ""
	CategoryForecastEdge       CategoryType = "forecast_edge"
	CategoryLaminar            CategoryType = "laminar"
	CategoryTurbulent          CategoryType = "turbulent"
	CategoryInertial           CategoryType = "inertial"
	CategoryViscous            CategoryType = "viscous"
	CategoryFrenzy             CategoryType = "frenzy"
	CategorySaturation         CategoryType = "saturation"
	CategoryOrganic            CategoryType = "organic"
	CategoryExhaustion         CategoryType = "exhaustion"
	CategoryHiddenAbsorption   CategoryType = "hidden_absorption"
	CategoryAggressiveDrive    CategoryType = "aggressive_drive"
	CategoryStochasticBalance  CategoryType = "stochastic_balance"
	CategoryVolumeStarvation   CategoryType = "volume_starvation"
	CategoryLoadedImbalance    CategoryType = "loaded_imbalance"
	CategorySpoofTrap          CategoryType = "spoof_trap"
	CategoryBookThinning       CategoryType = "book_thinning"
	CategoryDenseNeutrality    CategoryType = "dense_neutrality"
	CategoryInefficientLag     CategoryType = "inefficient_lag"
	CategorySynchronizedDrift  CategoryType = "synchronized_drift"
	CategoryDecoupledMove      CategoryType = "decoupled_move"
	CategoryAnchorStall        CategoryType = "anchor_stall"
	CategoryVerticalIgnition   CategoryType = "vertical_ignition"
	CategoryCoiledCompression  CategoryType = "coiled_compression"
	CategoryOrganicTrend       CategoryType = "organic_trend"
	CategoryFadedExhaustion    CategoryType = "faded_exhaustion"
	CategoryExtremeScarcity    CategoryType = "extreme_scarcity"
	CategoryMedianDepth        CategoryType = "median_depth"
	CategoryRobustLiquidity    CategoryType = "robust_liquidity"
	CategoryRiskOnSurge        CategoryType = "risk_on_surge"
	CategoryDivergentMove      CategoryType = "divergent_move"
	CategorySystemicSlump      CategoryType = "systemic_slump"
	CategoryLiquidityVacuum    CategoryType = "liquidity_vacuum"
	CategoryToxicBluff         CategoryType = "toxic_bluff"
	CategoryHardSupport        CategoryType = "hard_support"
	CategorySystemicHerd       CategoryType = "systemic_herd"
	CategoryDecoupledAlpha     CategoryType = "decoupled_alpha"
	CategoryStochasticNoise    CategoryType = "stochastic_noise"
	CategoryDivergentStress    CategoryType = "divergent_stress"
	CategoryEndogenousAlpha    CategoryType = "endogenous_alpha"
	CategorySystemicBeta       CategoryType = "systemic_beta"
	CategoryLiquidityShock     CategoryType = "liquidity_shock"
	CategoryCausalNoise        CategoryType = "causal_noise"
	CategoryMechanicalCollapse CategoryType = "mechanical_collapse"
	CategoryThermalExhaustion  CategoryType = "thermal_exhaustion"
	CategoryFragileExpansion   CategoryType = "fragile_expansion"
	CategoryActiveReversal     CategoryType = "active_reversal"
)

var Categories = map[int]CategoryType{
	0:  CategoryTypeNone,
	1:  CategoryForecastEdge,
	2:  CategoryLaminar,
	3:  CategoryTurbulent,
	4:  CategoryInertial,
	5:  CategoryViscous,
	6:  CategoryFrenzy,
	7:  CategorySaturation,
	8:  CategoryOrganic,
	9:  CategoryExhaustion,
	10: CategoryHiddenAbsorption,
	11: CategoryAggressiveDrive,
	12: CategoryStochasticBalance,
	13: CategoryVolumeStarvation,
	14: CategoryLoadedImbalance,
	15: CategorySpoofTrap,
	16: CategoryBookThinning,
	17: CategoryDenseNeutrality,
	18: CategoryInefficientLag,
	19: CategorySynchronizedDrift,
	20: CategoryDecoupledMove,
	21: CategoryAnchorStall,
	22: CategoryVerticalIgnition,
	23: CategoryCoiledCompression,
	24: CategoryOrganicTrend,
	25: CategoryFadedExhaustion,
	26: CategoryExtremeScarcity,
	27: CategoryMedianDepth,
	28: CategoryRobustLiquidity,
	29: CategoryRiskOnSurge,
	30: CategoryDivergentMove,
	31: CategorySystemicSlump,
	32: CategoryLiquidityVacuum,
	33: CategoryToxicBluff,
	34: CategoryHardSupport,
	35: CategorySystemicHerd,
	36: CategoryDecoupledAlpha,
	37: CategoryStochasticNoise,
	38: CategoryDivergentStress,
	39: CategoryEndogenousAlpha,
	40: CategorySystemicBeta,
	41: CategoryLiquidityShock,
	42: CategoryCausalNoise,
	43: CategoryMechanicalCollapse,
	44: CategoryThermalExhaustion,
	45: CategoryFragileExpansion,
	46: CategoryActiveReversal,
}

/*
Category names a measurement category. Thresholds belong on confidence and surprise
subjects inside comparison conditions, not on this type.
*/
type Category struct {
	Type CategoryType `yaml:"type" json:"type"`
}

func NewCategory(categoryType CategoryType) *Category {
	return &Category{Type: categoryType}
}
