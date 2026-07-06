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
	ForecastEdge       CategoryType = "forecast_edge"
	PhysicalField      CategoryType = "physical_field"
	Laminar            CategoryType = "laminar"
	Turbulent          CategoryType = "turbulent"
	Inertial           CategoryType = "inertial"
	Viscous            CategoryType = "viscous"
	Frenzy             CategoryType = "frenzy"
	Saturation         CategoryType = "saturation"
	Organic            CategoryType = "organic"
	Exhaustion         CategoryType = "exhaustion"
	HiddenAbsorption   CategoryType = "hidden_absorption"
	AggressiveDrive    CategoryType = "aggressive_drive"
	StochasticBalance  CategoryType = "stochastic_balance"
	VolumeStarvation   CategoryType = "volume_starvation"
	LoadedImbalance    CategoryType = "loaded_imbalance"
	SpoofTrap          CategoryType = "spoof_trap"
	BookThinning       CategoryType = "book_thinning"
	DenseNeutrality    CategoryType = "dense_neutrality"
	InefficientLag     CategoryType = "inefficient_lag"
	SynchronizedDrift  CategoryType = "synchronized_drift"
	DecoupledMove      CategoryType = "decoupled_move"
	AnchorStall        CategoryType = "anchor_stall"
	VerticalIgnition   CategoryType = "vertical_ignition"
	CoiledCompression  CategoryType = "coiled_compression"
	OrganicTrend       CategoryType = "organic_trend"
	FadedExhaustion    CategoryType = "faded_exhaustion"
	ExtremeScarcity    CategoryType = "extreme_scarcity"
	MedianDepth        CategoryType = "median_depth"
	RobustLiquidity    CategoryType = "robust_liquidity"
	RiskOnSurge        CategoryType = "risk_on_surge"
	DivergentMove      CategoryType = "divergent_move"
	SystemicSlump      CategoryType = "systemic_slump"
	LiquidityVacuum    CategoryType = "liquidity_vacuum"
	ToxicBluff         CategoryType = "toxic_bluff"
	HardSupport        CategoryType = "hard_support"
	SystemicHerd       CategoryType = "systemic_herd"
	DecoupledAlpha     CategoryType = "decoupled_alpha"
	StochasticNoise    CategoryType = "stochastic_noise"
	DivergentStress    CategoryType = "divergent_stress"
	EndogenousAlpha    CategoryType = "endogenous_alpha"
	SystemicBeta       CategoryType = "systemic_beta"
	LiquidityShock     CategoryType = "liquidity_shock"
	CausalNoise        CategoryType = "causal_noise"
	MechanicalCollapse CategoryType = "mechanical_collapse"
	ThermalExhaustion  CategoryType = "thermal_exhaustion"
	FragileExpansion   CategoryType = "fragile_expansion"
	ActiveReversal     CategoryType = "active_reversal"
	LaminarResonance   CategoryType = "laminar_resonance"
	TurbulentResonance CategoryType = "turbulent_resonance"
	Equilibrium        CategoryType = "equilibrium"
)

const (
	CategoryForecastEdge       CategoryType = ForecastEdge
	CategoryPhysicalField      CategoryType = PhysicalField
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
	PhysicalField,
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
