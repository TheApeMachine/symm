package types

type Category struct {
	Type       CategoryType `json:"type"`
	Confidence float64      `json:"confidence"`
	Surprisal  float64      `json:"surprisal"`
	Strength   float64      `json:"strength"`
}

type CategoryType string

const (
	CategoryTypeNone   CategoryType = ""
	ForecastEdge                    = "forecast_edge"
	Laminar                         = "laminar"
	Turbulent                       = "turbulent"
	Inertial                        = "inertial"
	Viscous                         = "viscous"
	Frenzy                          = "frenzy"
	Saturation                      = "saturation"
	Organic                         = "organic"
	Exhaustion                      = "exhaustion"
	HiddenAbsorption                = "hidden_absorption"
	AggressiveDrive                 = "aggressive_drive"
	StochasticBalance               = "stochastic_balance"
	VolumeStarvation                = "volume_starvation"
	LoadedImbalance                 = "loaded_imbalance"
	SpoofTrap                       = "spoof_trap"
	BookThinning                    = "book_thinning"
	DenseNeutrality                 = "dense_neutrality"
	InefficientLag                  = "inefficient_lag"
	SynchronizedDrift               = "synchronized_drift"
	DecoupledMove                   = "decoupled_move"
	AnchorStall                     = "anchor_stall"
	VerticalIgnition                = "vertical_ignition"
	CoiledCompression               = "coiled_compression"
	OrganicTrend                    = "organic_trend"
	FadedExhaustion                 = "faded_exhaustion"
	ExtremeScarcity                 = "extreme_scarcity"
	MedianDepth                     = "median_depth"
	RobustLiquidity                 = "robust_liquidity"
	RiskOnSurge                     = "risk_on_surge"
	DivergentMove                   = "divergent_move"
	SystemicSlump                   = "systemic_slump"
	LiquidityVacuum                 = "liquidity_vacuum"
	ToxicBluff                      = "toxic_bluff"
	HardSupport                     = "hard_support"
	SystemicHerd                    = "systemic_herd"
	DecoupledAlpha                  = "decoupled_alpha"
	StochasticNoise                 = "stochastic_noise"
	DivergentStress                 = "divergent_stress"
	EndogenousAlpha                 = "endogenous_alpha"
	SystemicBeta                    = "systemic_beta"
	LiquidityShock                  = "liquidity_shock"
	CausalNoise                     = "causal_noise"
	MechanicalCollapse              = "mechanical_collapse"
	ThermalExhaustion               = "thermal_exhaustion"
	FragileExpansion                = "fragile_expansion"
	ActiveReversal                  = "active_reversal"
	LaminarResonance                = "laminar_resonance"
	TurbulentResonance              = "turbulent_resonance"
	Equilibrium                     = "equilibrium"
)

const (
	CategoryForecastEdge       CategoryType = ForecastEdge
	CategoryLaminar            CategoryType = Laminar
	CategoryTurbulent          CategoryType = Turbulent
	CategoryInertial           CategoryType = Inertial
	CategoryViscous            CategoryType = Viscous
	CategoryFrenzy             CategoryType = Frenzy
	CategorySaturation         CategoryType = Saturation
	CategoryOrganic            CategoryType = Organic
	CategoryExhaustion         CategoryType = Exhaustion
	CategoryHiddenAbsorption   CategoryType = HiddenAbsorption
	CategoryAggressiveDrive    CategoryType = AggressiveDrive
	CategoryStochasticBalance  CategoryType = StochasticBalance
	CategoryVolumeStarvation   CategoryType = VolumeStarvation
	CategoryLoadedImbalance    CategoryType = LoadedImbalance
	CategorySpoofTrap          CategoryType = SpoofTrap
	CategoryBookThinning       CategoryType = BookThinning
	CategoryDenseNeutrality    CategoryType = DenseNeutrality
	CategoryInefficientLag     CategoryType = InefficientLag
	CategorySynchronizedDrift  CategoryType = SynchronizedDrift
	CategoryDecoupledMove      CategoryType = DecoupledMove
	CategoryAnchorStall        CategoryType = AnchorStall
	CategoryVerticalIgnition   CategoryType = VerticalIgnition
	CategoryCoiledCompression  CategoryType = CoiledCompression
	CategoryOrganicTrend       CategoryType = OrganicTrend
	CategoryFadedExhaustion    CategoryType = FadedExhaustion
	CategoryExtremeScarcity    CategoryType = ExtremeScarcity
	CategoryMedianDepth        CategoryType = MedianDepth
	CategoryRobustLiquidity    CategoryType = RobustLiquidity
	CategoryRiskOnSurge        CategoryType = RiskOnSurge
	CategoryDivergentMove      CategoryType = DivergentMove
	CategorySystemicSlump      CategoryType = SystemicSlump
	CategoryLiquidityVacuum    CategoryType = LiquidityVacuum
	CategoryToxicBluff         CategoryType = ToxicBluff
	CategoryHardSupport        CategoryType = HardSupport
	CategorySystemicHerd       CategoryType = SystemicHerd
	CategoryDecoupledAlpha     CategoryType = DecoupledAlpha
	CategoryStochasticNoise    CategoryType = StochasticNoise
	CategoryDivergentStress    CategoryType = DivergentStress
	CategoryEndogenousAlpha    CategoryType = EndogenousAlpha
	CategorySystemicBeta       CategoryType = SystemicBeta
	CategoryLiquidityShock     CategoryType = LiquidityShock
	CategoryCausalNoise        CategoryType = CausalNoise
	CategoryMechanicalCollapse CategoryType = MechanicalCollapse
	CategoryThermalExhaustion  CategoryType = ThermalExhaustion
	CategoryFragileExpansion   CategoryType = FragileExpansion
	CategoryActiveReversal     CategoryType = ActiveReversal
	CategoryLaminarResonance   CategoryType = LaminarResonance
	CategoryTurbulentResonance CategoryType = TurbulentResonance
	CategoryEquilibrium        CategoryType = Equilibrium
)

var categoryOrder = []CategoryType{
	ForecastEdge,
	Laminar,
	Turbulent,
	Inertial,
	Viscous,
	Frenzy,
	Saturation,
	Organic,
	Exhaustion,
	HiddenAbsorption,
	AggressiveDrive,
	StochasticBalance,
	VolumeStarvation,
	LoadedImbalance,
	SpoofTrap,
	BookThinning,
	DenseNeutrality,
	InefficientLag,
	SynchronizedDrift,
	DecoupledMove,
	AnchorStall,
	VerticalIgnition,
	CoiledCompression,
	OrganicTrend,
	FadedExhaustion,
	ExtremeScarcity,
	MedianDepth,
	RobustLiquidity,
	RiskOnSurge,
	DivergentMove,
	SystemicSlump,
	LiquidityVacuum,
	ToxicBluff,
	HardSupport,
	SystemicHerd,
	DecoupledAlpha,
	StochasticNoise,
	DivergentStress,
	EndogenousAlpha,
	SystemicBeta,
	LiquidityShock,
	CausalNoise,
	MechanicalCollapse,
	ThermalExhaustion,
	FragileExpansion,
	ActiveReversal,
	LaminarResonance,
	TurbulentResonance,
	Equilibrium,
}

func CategoryIndex(category CategoryType) int {
	for index, candidate := range categoryOrder {
		if candidate == category {
			return index + 1
		}
	}

	return 0
}

func CategoryByIndex(index int) CategoryType {
	if index <= 0 || index > len(categoryOrder) {
		return CategoryTypeNone
	}

	return categoryOrder[index-1]
}
