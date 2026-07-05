package types

type Category struct {
	Type       CategoryType `json:"type"`
	Confidence float64      `json:"confidence"`
	Surprisal  float64      `json:"surprisal"`
	Strength   float64      `json:"strength"`
}

type CategoryType string

const (
	ForecastEdge       = "forecast_edge"
	Laminar            = "laminar"
	Turbulent          = "turbulent"
	Inertial           = "inertial"
	Viscous            = "viscous"
	Frenzy             = "frenzy"
	Saturation         = "saturation"
	Organic            = "organic"
	Exhaustion         = "exhaustion"
	HiddenAbsorption   = "hidden_absorption"
	AggressiveDrive    = "aggressive_drive"
	StochasticBalance  = "stochastic_balance"
	VolumeStarvation   = "volume_starvation"
	LoadedImbalance    = "loaded_imbalance"
	SpoofTrap          = "spoof_trap"
	BookThinning       = "book_thinning"
	DenseNeutrality    = "dense_neutrality"
	InefficientLag     = "inefficient_lag"
	SynchronizedDrift  = "synchronized_drift"
	DecoupledMove      = "decoupled_move"
	AnchorStall        = "anchor_stall"
	VerticalIgnition   = "vertical_ignition"
	CoiledCompression  = "coiled_compression"
	OrganicTrend       = "organic_trend"
	FadedExhaustion    = "faded_exhaustion"
	ExtremeScarcity    = "extreme_scarcity"
	MedianDepth        = "median_depth"
	RobustLiquidity    = "robust_liquidity"
	RiskOnSurge        = "risk_on_surge"
	DivergentMove      = "divergent_move"
	SystemicSlump      = "systemic_slump"
	LiquidityVacuum    = "liquidity_vacuum"
	ToxicBluff         = "toxic_bluff"
	HardSupport        = "hard_support"
	SystemicHerd       = "systemic_herd"
	DecoupledAlpha     = "decoupled_alpha"
	StochasticNoise    = "stochastic_noise"
	DivergentStress    = "divergent_stress"
	EndogenousAlpha    = "endogenous_alpha"
	SystemicBeta       = "systemic_beta"
	LiquidityShock     = "liquidity_shock"
	CausalNoise        = "causal_noise"
	MechanicalCollapse = "mechanical_collapse"
	ThermalExhaustion  = "thermal_exhaustion"
	FragileExpansion   = "fragile_expansion"
	ActiveReversal     = "active_reversal"
	LaminarResonance   = "laminar_resonance"
	TurbulentResonance = "turbulent_resonance"
	Equilibrium        = "equilibrium"
)
