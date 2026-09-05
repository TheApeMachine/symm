package types

import "time"

type Category struct {
	At          time.Time    `json:"at"`
	Symbol      string       `json:"symbol,omitempty"`
	Type        CategoryType `json:"type"`
	Confidence  float64      `json:"confidence"`
	Surprisal   float64      `json:"surprisal"`
	Strength    float64      `json:"strength"`
	Maturity    float64      `json:"maturity,omitempty"`
	Uncertainty float64      `json:"uncertainty,omitempty"`
	Freshness   float64      `json:"freshness,omitempty"`
	Supporting  []string     `json:"supporting,omitempty"`
	Opposing    []string     `json:"opposing,omitempty"`
	Missing     []string     `json:"missing,omitempty"`
}

type CategoryType string

const (
	CategoryTypeNone       CategoryType = ""
	ForecastEdge           CategoryType = "forecast_edge"
	PhysicalField          CategoryType = "physical_field"
	Laminar                CategoryType = "laminar"
	Turbulent              CategoryType = "turbulent"
	Inertial               CategoryType = "inertial"
	Viscous                CategoryType = "viscous"
	Frenzy                 CategoryType = "frenzy"
	Saturation             CategoryType = "saturation"
	Organic                CategoryType = "organic"
	Exhaustion             CategoryType = "exhaustion"
	HiddenAbsorption       CategoryType = "hidden_absorption"
	AggressiveDrive        CategoryType = "aggressive_drive"
	StochasticBalance      CategoryType = "stochastic_balance"
	VolumeStarvation       CategoryType = "volume_starvation"
	LoadedImbalance        CategoryType = "loaded_imbalance"
	SpoofTrap              CategoryType = "spoof_trap"
	BookThinning           CategoryType = "book_thinning"
	DenseNeutrality        CategoryType = "dense_neutrality"
	InefficientLag         CategoryType = "inefficient_lag"
	SynchronizedDrift      CategoryType = "synchronized_drift"
	DecoupledMove          CategoryType = "decoupled_move"
	AnchorStall            CategoryType = "anchor_stall"
	VerticalIgnition       CategoryType = "vertical_ignition"
	CoiledCompression      CategoryType = "coiled_compression"
	OrganicTrend           CategoryType = "organic_trend"
	FadedExhaustion        CategoryType = "faded_exhaustion"
	ExtremeScarcity        CategoryType = "extreme_scarcity"
	MedianDepth            CategoryType = "median_depth"
	RobustLiquidity        CategoryType = "robust_liquidity"
	RiskOnSurge            CategoryType = "risk_on_surge"
	DivergentMove          CategoryType = "divergent_move"
	SystemicSlump          CategoryType = "systemic_slump"
	LeveragedIgnition      CategoryType = "leveraged_ignition"
	ShortSqueeze           CategoryType = "short_squeeze"
	AdverseLeverageBuildup CategoryType = "adverse_leverage_buildup"
	LongDeleveraging       CategoryType = "long_deleveraging"
	DerivativesDecoupling  CategoryType = "derivatives_decoupling"
	LiquidityVacuum        CategoryType = "liquidity_vacuum"
	ToxicBluff             CategoryType = "toxic_bluff"
	HardSupport            CategoryType = "hard_support"
	SystemicHerd           CategoryType = "systemic_herd"
	DecoupledAlpha         CategoryType = "decoupled_alpha"
	StochasticNoise        CategoryType = "stochastic_noise"
	DivergentStress        CategoryType = "divergent_stress"
	EndogenousAlpha        CategoryType = "endogenous_alpha"
	SystemicBeta           CategoryType = "systemic_beta"
	LiquidityShock         CategoryType = "liquidity_shock"
	CausalNoise            CategoryType = "causal_noise"
	MechanicalCollapse     CategoryType = "mechanical_collapse"
	ThermalExhaustion      CategoryType = "thermal_exhaustion"
	FragileExpansion       CategoryType = "fragile_expansion"
	ActiveReversal         CategoryType = "active_reversal"
	LaminarResonance       CategoryType = "laminar_resonance"
	TurbulentResonance     CategoryType = "turbulent_resonance"
	Equilibrium            CategoryType = "equilibrium"
)

const (
	CategoryForecastEdge           CategoryType = ForecastEdge
	CategoryPhysicalField          CategoryType = PhysicalField
	CategoryLaminar                CategoryType = Laminar
	CategoryTurbulent              CategoryType = Turbulent
	CategoryInertial               CategoryType = Inertial
	CategoryViscous                CategoryType = Viscous
	CategoryFrenzy                 CategoryType = Frenzy
	CategorySaturation             CategoryType = Saturation
	CategoryOrganic                CategoryType = Organic
	CategoryExhaustion             CategoryType = Exhaustion
	CategoryHiddenAbsorption       CategoryType = HiddenAbsorption
	CategoryAggressiveDrive        CategoryType = AggressiveDrive
	CategoryStochasticBalance      CategoryType = StochasticBalance
	CategoryVolumeStarvation       CategoryType = VolumeStarvation
	CategoryLoadedImbalance        CategoryType = LoadedImbalance
	CategorySpoofTrap              CategoryType = SpoofTrap
	CategoryBookThinning           CategoryType = BookThinning
	CategoryDenseNeutrality        CategoryType = DenseNeutrality
	CategoryInefficientLag         CategoryType = InefficientLag
	CategorySynchronizedDrift      CategoryType = SynchronizedDrift
	CategoryDecoupledMove          CategoryType = DecoupledMove
	CategoryAnchorStall            CategoryType = AnchorStall
	CategoryVerticalIgnition       CategoryType = VerticalIgnition
	CategoryCoiledCompression      CategoryType = CoiledCompression
	CategoryOrganicTrend           CategoryType = OrganicTrend
	CategoryFadedExhaustion        CategoryType = FadedExhaustion
	CategoryExtremeScarcity        CategoryType = ExtremeScarcity
	CategoryMedianDepth            CategoryType = MedianDepth
	CategoryRobustLiquidity        CategoryType = RobustLiquidity
	CategoryRiskOnSurge            CategoryType = RiskOnSurge
	CategoryDivergentMove          CategoryType = DivergentMove
	CategorySystemicSlump          CategoryType = SystemicSlump
	CategoryLeveragedIgnition      CategoryType = LeveragedIgnition
	CategoryShortSqueeze           CategoryType = ShortSqueeze
	CategoryAdverseLeverageBuildup CategoryType = AdverseLeverageBuildup
	CategoryLongDeleveraging       CategoryType = LongDeleveraging
	CategoryDerivativesDecoupling  CategoryType = DerivativesDecoupling
	CategoryLiquidityVacuum        CategoryType = LiquidityVacuum
	CategoryToxicBluff             CategoryType = ToxicBluff
	CategoryHardSupport            CategoryType = HardSupport
	CategorySystemicHerd           CategoryType = SystemicHerd
	CategoryDecoupledAlpha         CategoryType = DecoupledAlpha
	CategoryStochasticNoise        CategoryType = StochasticNoise
	CategoryDivergentStress        CategoryType = DivergentStress
	CategoryEndogenousAlpha        CategoryType = EndogenousAlpha
	CategorySystemicBeta           CategoryType = SystemicBeta
	CategoryLiquidityShock         CategoryType = LiquidityShock
	CategoryCausalNoise            CategoryType = CausalNoise
	CategoryMechanicalCollapse     CategoryType = MechanicalCollapse
	CategoryThermalExhaustion      CategoryType = ThermalExhaustion
	CategoryFragileExpansion       CategoryType = FragileExpansion
	CategoryActiveReversal         CategoryType = ActiveReversal
	CategoryLaminarResonance       CategoryType = LaminarResonance
	CategoryTurbulentResonance     CategoryType = TurbulentResonance
	CategoryEquilibrium            CategoryType = Equilibrium
)

var CategoryOrder = []CategoryType{
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
	LeveragedIgnition,
	ShortSqueeze,
	AdverseLeverageBuildup,
	LongDeleveraging,
	DerivativesDecoupling,
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
	for index, candidate := range CategoryOrder {
		if candidate == category {
			return index + 1
		}
	}

	return 0
}

/*
CategoryOrderLess reports whether left precedes right in the stable category
vocabulary order. It is the deterministic tie-break the category solver uses to
resolve equal-evidence regimes. A category absent from CategoryOrder sorts after
every listed category, preserving a stable extension for future categories.
*/
func CategoryOrderLess(left CategoryType, right CategoryType) bool {
	return CategoryIndex(left) < CategoryIndex(right)
}

/*
CategorySchema declares which measured value is fed to a category input. It
carries identity only: the signal source and the metric name string exactly as
the Measurement.Metrics map keys it (including any ":side" suffix). The
nomagique classifier owns extraction and classification.
*/
type CategorySchema struct {
	Source   SourceType
	Metric   string
	Category CategoryType
}

/*
CategorySchemas maps signal metric names to the category evidence they
represent. Keys are the exact metric name strings the signals emit — never a
reconstructed enum — so the category solver reads Measurement.Metrics directly
without inventing identity. Repeated categories are combined by the geometric
mean: a conjunction where any weak leg drags the composite down, so a category
carries mass only when the whole complex agrees. Absent legs contribute
nothing. Ties break by first appearance, so corroborated rows follow the
single-axis rows they extend.

VerticalIgnition is the corroborated ignition complex: PumpDump's volume
breakout anchors it, with Hawkes' near-critical cascade clustering, DepthFlow's
hollow ask book, and CVD's aggressive drive as corroborating legs.
*/
var CategorySchemas = []CategorySchema{
	// Correlation: dependence structure across the pair/cohort.
	{Source: SourceCorrelation, Metric: "signed_correlation", Category: SystemicHerd},
	{Source: SourceCorrelation, Metric: "cohort_signed_correlation", Category: SystemicHerd},
	{Source: SourceCorrelation, Metric: "cohort_absolute_correlation", Category: SystemicHerd},
	{Source: SourceCorrelation, Metric: "absolute_correlation", Category: SystemicHerd},
	{Source: SourceCorrelation, Metric: "covariance", Category: SystemicHerd},
	{Source: SourceCorrelation, Metric: "cohort_peer_count", Category: SystemicHerd},
	{Source: SourceCorrelation, Metric: "cohort_effective_peer_count", Category: SystemicHerd},
	{Source: SourceCorrelation, Metric: "relative_return_energy_zscore", Category: DecoupledAlpha},
	{Source: SourceCorrelation, Metric: "relative_return_energy_divergence", Category: DecoupledAlpha},
	{Source: SourceCorrelation, Metric: "relative_cohort_return_energy", Category: DecoupledAlpha},
	{Source: SourceCorrelation, Metric: "relative_return_energy_baseline", Category: DecoupledAlpha},
	{Source: SourceCorrelation, Metric: "relative_return_energy_velocity", Category: DecoupledAlpha},
	{Source: SourceCorrelation, Metric: "focal_return_energy_rate", Category: DecoupledAlpha},
	{Source: SourceCorrelation, Metric: "peer_return_energy_rate", Category: DecoupledAlpha},
	{Source: SourceCorrelation, Metric: "correlation_zscore", Category: StochasticNoise},
	{Source: SourceCorrelation, Metric: "correlation_p_value", Category: StochasticNoise},
	{Source: SourceCorrelation, Metric: "correlation_standard_error_fisher", Category: StochasticNoise},
	{Source: SourceCorrelation, Metric: "correlation_baseline", Category: StochasticNoise},
	{Source: SourceCorrelation, Metric: "correlation_divergence", Category: DivergentStress},
	{Source: SourceCorrelation, Metric: "cohort_correlation_dispersion", Category: DivergentStress},
	{Source: SourceCorrelation, Metric: "correlation_velocity", Category: DivergentStress},
	{Source: SourceCorrelation, Metric: "return_energy_rate:measured", Category: DivergentStress},
	{Source: SourceCorrelation, Metric: "return_energy_rate:reference", Category: DivergentStress},
	{Source: SourceCorrelation, Metric: "return_energy:measured", Category: PhysicalField},
	{Source: SourceCorrelation, Metric: "return_energy:reference", Category: PhysicalField},
	{Source: SourceCorrelation, Metric: "overlap_density", Category: CausalNoise},
	{Source: SourceCorrelation, Metric: "overlap_pair_count", Category: CausalNoise},
	{Source: SourceCorrelation, Metric: "shared_time", Category: CausalNoise},
	{Source: SourceCorrelation, Metric: "supported_return_count:measured", Category: CausalNoise},
	{Source: SourceCorrelation, Metric: "supported_return_count:reference", Category: CausalNoise},
	{Source: SourceCorrelation, Metric: "effective_sample_count", Category: CausalNoise},
	{Source: SourceCorrelation, Metric: "observation_count", Category: CausalNoise},
	{Source: SourceCorrelation, Metric: "last_price", Category: Equilibrium},

	// Lead-lag: temporal leadership / alignment between pairs.
	{Source: SourceLeadLag, Metric: "correlation_gain_zscore", Category: InefficientLag},
	{Source: SourceLeadLag, Metric: "absolute_correlation_gain", Category: InefficientLag},
	{Source: SourceLeadLag, Metric: "lag_peak_prominence", Category: InefficientLag},
	{Source: SourceLeadLag, Metric: "correlation_gain_baseline", Category: InefficientLag},
	{Source: SourceLeadLag, Metric: "lag_baseline_seconds", Category: InefficientLag},
	{Source: SourceLeadLag, Metric: "search_adjusted_p_value", Category: InefficientLag},
	{Source: SourceLeadLag, Metric: "correlation_p_value", Category: InefficientLag},
	{Source: SourceLeadLag, Metric: "contemporaneous_correlation", Category: SynchronizedDrift},
	{Source: SourceLeadLag, Metric: "best_lag_correlation", Category: SynchronizedDrift},
	{Source: SourceLeadLag, Metric: "best_lag_correlation_baseline", Category: SynchronizedDrift},
	{Source: SourceLeadLag, Metric: "lag_zscore", Category: DecoupledMove},
	{Source: SourceLeadLag, Metric: "lag_divergence_seconds", Category: DecoupledMove},
	{Source: SourceLeadLag, Metric: "best_lag_index", Category: DecoupledMove},
	{Source: SourceLeadLag, Metric: "best_lag_correlation_zscore", Category: AnchorStall},
	{Source: SourceLeadLag, Metric: "lag_velocity", Category: AnchorStall},
	{Source: SourceLeadLag, Metric: "lag_divergence_seconds", Category: DivergentMove},
	{Source: SourceLeadLag, Metric: "correlation_gain_velocity", Category: Turbulent},
	{Source: SourceLeadLag, Metric: "lag_noise_scale_seconds", Category: CausalNoise},
	{Source: SourceLeadLag, Metric: "effective_sample_count", Category: CausalNoise},
	{Source: SourceLeadLag, Metric: "observation_count", Category: CausalNoise},
	{Source: SourceLeadLag, Metric: "overlap_pair_count", Category: CausalNoise},
	{Source: SourceLeadLag, Metric: "measured_return_count", Category: CausalNoise},
	{Source: SourceLeadLag, Metric: "reference_return_count", Category: CausalNoise},
	{Source: SourceLeadLag, Metric: "search_count", Category: CausalNoise},
	{Source: SourceLeadLag, Metric: "lag_search_resolution_seconds", Category: CausalNoise},
	{Source: SourceLeadLag, Metric: "lag_search_span", Category: CausalNoise},
	{Source: SourceLeadLag, Metric: "last_price", Category: Equilibrium},

	// CVD / executed flow: aggressive execution economics and midpoint response.
	{Source: SourceCVD, Metric: "signed_net_fraction_zscore", Category: AggressiveDrive},
	{Source: SourceCVD, Metric: "signed_net_fraction_divergence", Category: AggressiveDrive},
	{Source: SourceCVD, Metric: "cumulative_notional_delta", Category: AggressiveDrive},
	{Source: SourceCVD, Metric: "cumulative_volume_delta", Category: AggressiveDrive},
	{Source: SourceCVD, Metric: "aggressive_notional:buy", Category: AggressiveDrive},
	{Source: SourceCVD, Metric: "aggressive_notional:sell", Category: AggressiveDrive},
	{Source: SourceCVD, Metric: "net_executed_quantity", Category: AggressiveDrive},
	{Source: SourceCVD, Metric: "net_notional", Category: AggressiveDrive},
	{Source: SourceCVD, Metric: "gross_notional_rate_divergence", Category: VolumeStarvation},
	{Source: SourceCVD, Metric: "trade_rate", Category: VolumeStarvation},
	{Source: SourceCVD, Metric: "gross_notional", Category: VolumeStarvation},
	{Source: SourceCVD, Metric: "gross_executed_quantity", Category: VolumeStarvation},
	{Source: SourceCVD, Metric: "mean_trade_notional", Category: VolumeStarvation},
	{Source: SourceCVD, Metric: "midpoint_return_rate_zscore", Category: StochasticBalance},
	{Source: SourceCVD, Metric: "flow_aligned_midpoint_return", Category: StochasticBalance},
	{Source: SourceCVD, Metric: "signed_count_fraction", Category: StochasticBalance},
	{Source: SourceCVD, Metric: "trade_count", Category: StochasticBalance},
	{Source: SourceCVD, Metric: "trade_count:buy", Category: StochasticBalance},
	{Source: SourceCVD, Metric: "trade_count:sell", Category: StochasticBalance},
	{Source: SourceCVD, Metric: "executed_quantity:buy", Category: StochasticBalance},
	{Source: SourceCVD, Metric: "executed_quantity:sell", Category: StochasticBalance},
	{Source: SourceCVD, Metric: "midpoint_log_return", Category: StochasticBalance},
	{Source: SourceCVD, Metric: "cvd_epoch_from", Category: CausalNoise},

	// Hawkes: arrival/excitation dynamics.
	{Source: SourceHawkes, Metric: "branching_spectral_radius", Category: Turbulent},
	{Source: SourceHawkes, Metric: "log_likelihood_gain_vs_poisson", Category: Turbulent},
	{Source: SourceHawkes, Metric: "log_likelihood_per_event:hawkes", Category: Turbulent},
	{Source: SourceHawkes, Metric: "excitation_share:buy", Category: Frenzy},
	{Source: SourceHawkes, Metric: "excitation_share:sell", Category: Frenzy},
	{Source: SourceHawkes, Metric: "conditional_intensity:buy", Category: Frenzy},
	{Source: SourceHawkes, Metric: "conditional_intensity:sell", Category: Frenzy},
	{Source: SourceHawkes, Metric: "count_innovation:buy", Category: Frenzy},
	{Source: SourceHawkes, Metric: "count_innovation:sell", Category: Frenzy},
	{Source: SourceHawkes, Metric: "excitation_intensity:buy", Category: Frenzy},
	{Source: SourceHawkes, Metric: "excitation_intensity:sell", Category: Frenzy},
	{Source: SourceHawkes, Metric: "excitation_mass:buy", Category: Frenzy},
	{Source: SourceHawkes, Metric: "excitation_mass:sell", Category: Frenzy},
	{Source: SourceHawkes, Metric: "arrival_rate", Category: Inertial},
	{Source: SourceHawkes, Metric: "background_rate", Category: Inertial},
	{Source: SourceHawkes, Metric: "background_rate:buy", Category: Inertial},
	{Source: SourceHawkes, Metric: "background_rate:sell", Category: Inertial},
	{Source: SourceHawkes, Metric: "arrival_rate:buy", Category: Inertial},
	{Source: SourceHawkes, Metric: "arrival_rate:sell", Category: Inertial},
	{Source: SourceHawkes, Metric: "event_count", Category: Inertial},
	{Source: SourceHawkes, Metric: "offspring:sell_from_sell", Category: MechanicalCollapse},
	{Source: SourceHawkes, Metric: "offspring:buy_from_buy", Category: MechanicalCollapse},
	{Source: SourceHawkes, Metric: "compensator:buy", Category: MechanicalCollapse},
	{Source: SourceHawkes, Metric: "compensator:sell", Category: MechanicalCollapse},
	{Source: SourceHawkes, Metric: "offspring:sell_from_buy", Category: ActiveReversal},
	{Source: SourceHawkes, Metric: "offspring:buy_from_sell", Category: ActiveReversal},
	{Source: SourceHawkes, Metric: "excitation_decay:sell_from_buy", Category: ActiveReversal},
	{Source: SourceHawkes, Metric: "excitation_decay:buy_from_sell", Category: ActiveReversal},
	{Source: SourceHawkes, Metric: "excitation_timescale:sell_from_buy", Category: ActiveReversal},
	{Source: SourceHawkes, Metric: "excitation_timescale:buy_from_sell", Category: ActiveReversal},
	{Source: SourceHawkes, Metric: "excitation_decay:buy_from_buy", Category: ThermalExhaustion},
	{Source: SourceHawkes, Metric: "excitation_decay:sell_from_sell", Category: ThermalExhaustion},
	{Source: SourceHawkes, Metric: "excitation_timescale:buy_from_buy", Category: ThermalExhaustion},
	{Source: SourceHawkes, Metric: "excitation_timescale:sell_from_sell", Category: ThermalExhaustion},
	{Source: SourceHawkes, Metric: "excitation_amplitude:buy_from_buy", Category: Frenzy},
	{Source: SourceHawkes, Metric: "excitation_amplitude:sell_from_sell", Category: Frenzy},
	{Source: SourceHawkes, Metric: "log_likelihood:poisson", Category: CausalNoise},
	{Source: SourceHawkes, Metric: "log_likelihood:self_only", Category: CausalNoise},
	{Source: SourceHawkes, Metric: "log_likelihood:hawkes", Category: CausalNoise},
	{Source: SourceHawkes, Metric: "log_likelihood_gain_vs_self_only", Category: CausalNoise},
	{Source: SourceHawkes, Metric: "log_likelihood_gain_per_event_vs_self_only", Category: CausalNoise},
	{Source: SourceHawkes, Metric: "excitation_share", Category: StochasticNoise},
	{Source: SourceHawkes, Metric: "event_count:buy", Category: StochasticNoise},
	{Source: SourceHawkes, Metric: "event_count:sell", Category: StochasticNoise},

	// DepthFlow: facts present in each Level-3 mutation.
	{Source: SourceDepthFlow, Metric: "observed_notional_imbalance_zscore", Category: LoadedImbalance},
	{Source: SourceDepthFlow, Metric: "observed_notional_imbalance_divergence", Category: LoadedImbalance},
	{Source: SourceDepthFlow, Metric: "observed_notional_imbalance", Category: LoadedImbalance},
	{Source: SourceDepthFlow, Metric: "observed_notional_imbalance_baseline", Category: LoadedImbalance},
	{Source: SourceDepthFlow, Metric: "add_notional:bid", Category: LoadedImbalance},
	{Source: SourceDepthFlow, Metric: "add_notional:ask", Category: LoadedImbalance},
	{Source: SourceDepthFlow, Metric: "observed_notional_rate_zscore", Category: DenseNeutrality},
	{Source: SourceDepthFlow, Metric: "observed_notional_rate", Category: DenseNeutrality},
	{Source: SourceDepthFlow, Metric: "observed_notional_rate_baseline", Category: DenseNeutrality},
	{Source: SourceDepthFlow, Metric: "mutation_activity_imbalance", Category: BookThinning},
	{Source: SourceDepthFlow, Metric: "delete_count:bid", Category: BookThinning},
	{Source: SourceDepthFlow, Metric: "delete_count:ask", Category: BookThinning},
	{Source: SourceDepthFlow, Metric: "mutation_count:bid", Category: BookThinning},
	{Source: SourceDepthFlow, Metric: "mutation_count:ask", Category: BookThinning},
	{Source: SourceDepthFlow, Metric: "observed_notional_rate_divergence", Category: BookThinning},
	{Source: SourceDepthFlow, Metric: "observed_notional", Category: RobustLiquidity},
	{Source: SourceDepthFlow, Metric: "observed_notional:bid", Category: ExtremeScarcity},
	{Source: SourceDepthFlow, Metric: "observed_notional:ask", Category: ExtremeScarcity},

	// PumpDump: volume-clock ignition, spread compression, and midpoint return.
	{Source: SourcePumpDump, Metric: "volume_bar_quantity", Category: VerticalIgnition},
	{Source: SourcePumpDump, Metric: "volume_rate", Category: VerticalIgnition},
	{Source: SourcePumpDump, Metric: "midpoint_return_rate_zscore", Category: VerticalIgnition},
	{Source: SourcePumpDump, Metric: "notional_rate_divergence", Category: VerticalIgnition},
	{Source: SourcePumpDump, Metric: "volume_bar_notional", Category: VerticalIgnition},
	{Source: SourcePumpDump, Metric: "volume_bar_quantity", Category: CoiledCompression},
	{Source: SourcePumpDump, Metric: "relative_spread", Category: CoiledCompression},
	{Source: SourcePumpDump, Metric: "spread_zscore", Category: CoiledCompression},
	{Source: SourcePumpDump, Metric: "volume_bar_duration", Category: CoiledCompression},
	{Source: SourcePumpDump, Metric: "relative_spread_baseline", Category: CoiledCompression},
	{Source: SourcePumpDump, Metric: "spread", Category: CoiledCompression},
	{Source: SourcePumpDump, Metric: "trade_interval_seconds", Category: OrganicTrend},
	{Source: SourcePumpDump, Metric: "midpoint_return_zscore", Category: OrganicTrend},
	{Source: SourcePumpDump, Metric: "notional_rate_baseline", Category: OrganicTrend},
	{Source: SourcePumpDump, Metric: "volume_bar_target_quantity", Category: OrganicTrend},
	{Source: SourcePumpDump, Metric: "volume_bar_trade_count", Category: OrganicTrend},
	{Source: SourcePumpDump, Metric: "midpoint_return_zscore", Category: FadedExhaustion},
	{Source: SourcePumpDump, Metric: "midpoint_return_rate_divergence", Category: FadedExhaustion},
	{Source: SourcePumpDump, Metric: "trade_notional", Category: FadedExhaustion},
	{Source: SourcePumpDump, Metric: "trade_quantity", Category: FadedExhaustion},
	{Source: SourcePumpDump, Metric: "notional_rate_zscore", Category: VerticalIgnition},
	{Source: SourcePumpDump, Metric: "trade_price", Category: Equilibrium},
	{Source: SourcePumpDump, Metric: "best_ask", Category: Equilibrium},
	{Source: SourcePumpDump, Metric: "best_bid", Category: Equilibrium},
	{Source: SourcePumpDump, Metric: "midpoint", Category: Equilibrium},
	{Source: SourcePumpDump, Metric: "midpoint:at", Category: Equilibrium},
	{Source: SourcePumpDump, Metric: "midpoint:from", Category: Equilibrium},

	// Liquidity: displayed executable capacity and spread.
	{Source: SourceLiquidity, Metric: "relative_spread", Category: ExtremeScarcity},
	{Source: SourceLiquidity, Metric: "depth_zscore:ask", Category: ExtremeScarcity},
	{Source: SourceLiquidity, Metric: "depth_zscore:bid", Category: ExtremeScarcity},
	{Source: SourceLiquidity, Metric: "depth_divergence:ask", Category: ExtremeScarcity},
	{Source: SourceLiquidity, Metric: "depth_divergence:bid", Category: ExtremeScarcity},
	{Source: SourceLiquidity, Metric: "spread_noise_scale", Category: ExtremeScarcity},
	{Source: SourceLiquidity, Metric: "two_sided_touch_notional", Category: RobustLiquidity},
	{Source: SourceLiquidity, Metric: "touch_notional:ask", Category: RobustLiquidity},
	{Source: SourceLiquidity, Metric: "touch_notional_baseline:bid", Category: RobustLiquidity},
	{Source: SourceLiquidity, Metric: "touch_notional_baseline:ask", Category: RobustLiquidity},
	{Source: SourceLiquidity, Metric: "depth_ratio:ask", Category: RobustLiquidity},
	{Source: SourceLiquidity, Metric: "touch_quantity:ask", Category: RobustLiquidity},
	{Source: SourceLiquidity, Metric: "divergence_velocity:bid", Category: BookThinning},
	{Source: SourceLiquidity, Metric: "divergence_velocity:ask", Category: BookThinning},
	{Source: SourceLiquidity, Metric: "spread_divergence_velocity", Category: BookThinning},
	{Source: SourceLiquidity, Metric: "depth_noise_scale:bid", Category: StochasticNoise},
	{Source: SourceLiquidity, Metric: "depth_noise_scale:ask", Category: StochasticNoise},
	{Source: SourceLiquidity, Metric: "divergence_velocity_snr:bid", Category: StochasticNoise},
	{Source: SourceLiquidity, Metric: "divergence_velocity_snr:ask", Category: StochasticNoise},
	{Source: SourceLiquidity, Metric: "spread_divergence_velocity_snr", Category: StochasticNoise},
	{Source: SourceLiquidity, Metric: "spread", Category: Equilibrium},
	{Source: SourceLiquidity, Metric: "midpoint", Category: Equilibrium},
	{Source: SourceLiquidity, Metric: "best_ask_price", Category: Equilibrium},
	{Source: SourceLiquidity, Metric: "best_bid_price", Category: Equilibrium},

	// Sentiment: cross-sectional return state.
	{Source: SourceSentiment, Metric: "directional_consensus", Category: RiskOnSurge},
	{Source: SourceSentiment, Metric: "directional_agreement", Category: RiskOnSurge},
	{Source: SourceSentiment, Metric: "advance_count", Category: RiskOnSurge},
	{Source: SourceSentiment, Metric: "same_direction_peer_count", Category: RiskOnSurge},
	{Source: SourceSentiment, Metric: "largest_signed_return", Category: RiskOnSurge},
	{Source: SourceSentiment, Metric: "median_return", Category: RiskOnSurge},
	{Source: SourceSentiment, Metric: "median_absolute_return_zscore", Category: DivergentMove},
	{Source: SourceSentiment, Metric: "return_dispersion_zscore", Category: DivergentMove},
	{Source: SourceSentiment, Metric: "breadth_zscore", Category: SystemicSlump},
	{Source: SourceSentiment, Metric: "decline_count", Category: SystemicSlump},
	{Source: SourceSentiment, Metric: "decline_fraction", Category: SystemicSlump},
	{Source: SourceSentiment, Metric: "opposite_direction_peer_fraction", Category: SystemicSlump},
	{Source: SourceSentiment, Metric: "unchanged_fraction", Category: SystemicHerd},
	{Source: SourceSentiment, Metric: "zero_return_peer_fraction", Category: SystemicHerd},
	{Source: SourceSentiment, Metric: "valid_member_count", Category: SystemicHerd},
	{Source: SourceSentiment, Metric: "cohort_member_count", Category: SystemicHerd},
	{Source: SourceSentiment, Metric: "excluded_member_count", Category: SystemicHerd},
	{Source: SourceSentiment, Metric: "rms_return", Category: SystemicBeta},
	{Source: SourceSentiment, Metric: "mean_absolute_return", Category: SystemicBeta},
	{Source: SourceSentiment, Metric: "median_absolute_return", Category: SystemicBeta},
	{Source: SourceSentiment, Metric: "magnitude_mad", Category: SystemicBeta},
	{Source: SourceSentiment, Metric: "largest_move_mad_excess", Category: EndogenousAlpha},
	{Source: SourceSentiment, Metric: "largest_move_ratio_zscore", Category: EndogenousAlpha},
	{Source: SourceSentiment, Metric: "largest_move_share_zscore", Category: EndogenousAlpha},
	{Source: SourceSentiment, Metric: "peer_median_absolute_return", Category: EndogenousAlpha},
	{Source: SourceSentiment, Metric: "peer_magnitude_mad", Category: EndogenousAlpha},
	{Source: SourceSentiment, Metric: "largest_move_ratio", Category: DecoupledAlpha},
	{Source: SourceSentiment, Metric: "largest_move_ratio_baseline", Category: DecoupledAlpha},
	{Source: SourceSentiment, Metric: "largest_move_share_baseline", Category: DecoupledAlpha},
	{Source: SourceSentiment, Metric: "largest_absolute_return", Category: DecoupledAlpha},
	{Source: SourceSentiment, Metric: "return", Category: DecoupledAlpha},
	{Source: SourceSentiment, Metric: "return_dispersion_ratio", Category: DivergentStress},
	{Source: SourceSentiment, Metric: "return_dispersion_velocity", Category: DivergentStress},
	{Source: SourceSentiment, Metric: "return_dispersion_baseline", Category: DivergentStress},
	{Source: SourceSentiment, Metric: "return_interquartile_range", Category: DivergentStress},
	{Source: SourceSentiment, Metric: "return_mad", Category: DivergentStress},
	{Source: SourceSentiment, Metric: "median_absolute_return_ratio", Category: FragileExpansion},
	{Source: SourceSentiment, Metric: "absolute_return", Category: FragileExpansion},
	{Source: SourceSentiment, Metric: "largest_move_tie_count", Category: FragileExpansion},
	{Source: SourceSentiment, Metric: "zero_return_peer_count", Category: Equilibrium},
	{Source: SourceSentiment, Metric: "unchanged_count", Category: Equilibrium},
	{Source: SourceSentiment, Metric: "opposite_direction_peer_count", Category: Equilibrium},
	{Source: SourceSentiment, Metric: "asof_age_seconds", Category: CausalNoise},
	{Source: SourceSentiment, Metric: "from_age_seconds", Category: CausalNoise},
	{Source: SourceSentiment, Metric: "cohort_horizon_seconds", Category: CausalNoise},
	{Source: SourceSentiment, Metric: "median_asof_age_seconds", Category: CausalNoise},
	{Source: SourceSentiment, Metric: "median_from_age_seconds", Category: CausalNoise},
	{Source: SourceSentiment, Metric: "max_asof_age_seconds", Category: CausalNoise},

	// Derivatives: leverage, liquidation, and basis.
	{Source: SourceDerivatives, Metric: "open_interest_growth_zscore", Category: LeveragedIgnition},
	{Source: SourceDerivatives, Metric: "open_interest_growth_rate", Category: LeveragedIgnition},
	{Source: SourceDerivatives, Metric: "open_interest_change", Category: LeveragedIgnition},
	{Source: SourceDerivatives, Metric: "open_interest_log_change", Category: LeveragedIgnition},
	{Source: SourceDerivatives, Metric: "liquidation_notional_rate", Category: AdverseLeverageBuildup},
	{Source: SourceDerivatives, Metric: "liquidation_share_velocity", Category: AdverseLeverageBuildup},
	{Source: SourceDerivatives, Metric: "net_liquidation_notional", Category: LongDeleveraging},
	{Source: SourceDerivatives, Metric: "liquidation_share", Category: LongDeleveraging},
	{Source: SourceDerivatives, Metric: "basis_zscore", Category: DerivativesDecoupling},
	{Source: SourceDerivatives, Metric: "basis_rate", Category: DerivativesDecoupling},
	{Source: SourceDerivatives, Metric: "basis_closure_error", Category: DerivativesDecoupling},
	{Source: SourceDerivatives, Metric: "open_interest_growth_baseline", Category: DerivativesDecoupling},
	{Source: SourceDerivatives, Metric: "derivative_price", Category: Equilibrium},
	{Source: SourceDerivatives, Metric: "reference_price", Category: Equilibrium},

	// Toxicity: liquidity disposition.
	{Source: SourceToxicity, Metric: "fill_fraction_zscore:bid", Category: LiquidityVacuum},
	{Source: SourceToxicity, Metric: "fill_fraction_zscore:ask", Category: LiquidityVacuum},
	{Source: SourceToxicity, Metric: "retreat_fraction_zscore:bid", Category: LiquidityVacuum},
	{Source: SourceToxicity, Metric: "retreat_fraction_zscore:ask", Category: LiquidityVacuum},
	{Source: SourceToxicity, Metric: "net_withdrawal_fraction:ask", Category: LiquidityVacuum},
	{Source: SourceToxicity, Metric: "withdrawal_fraction_divergence:bid", Category: LiquidityVacuum},
	{Source: SourceToxicity, Metric: "withdrawal_fraction_divergence:ask", Category: LiquidityVacuum},
	{Source: SourceToxicity, Metric: "withdrawal_fraction_baseline:ask", Category: LiquidityVacuum},
	{Source: SourceToxicity, Metric: "withdrawal_fraction_baseline:bid", Category: LiquidityVacuum},
	{Source: SourceToxicity, Metric: "net_withdrawal_rate:ask", Category: LiquidityVacuum},
	{Source: SourceToxicity, Metric: "net_withdrawn_quantity:ask", Category: LiquidityVacuum},
	{Source: SourceToxicity, Metric: "net_withdrawn_quantity:bid", Category: LiquidityVacuum},
	{Source: SourceToxicity, Metric: "retreat_fraction_baseline:ask", Category: LiquidityShock},
	{Source: SourceToxicity, Metric: "retreat_fraction_baseline:bid", Category: LiquidityShock},
	{Source: SourceToxicity, Metric: "retreated_quantity:bid", Category: LiquidityShock},
	{Source: SourceToxicity, Metric: "retreated_quantity:ask", Category: LiquidityShock},
	{Source: SourceToxicity, Metric: "retreat_rate:bid", Category: LiquidityShock},
	{Source: SourceToxicity, Metric: "retreat_rate:ask", Category: LiquidityShock},
	{Source: SourceToxicity, Metric: "retreat_fraction:bid", Category: LiquidityShock},
	{Source: SourceToxicity, Metric: "retreat_fraction:ask", Category: LiquidityShock},
	{Source: SourceToxicity, Metric: "net_replenished_quantity:ask", Category: HardSupport},
	{Source: SourceToxicity, Metric: "net_replenishment_rate:ask", Category: HardSupport},
	{Source: SourceToxicity, Metric: "previous_touch_quantity:bid", Category: HardSupport},
	{Source: SourceToxicity, Metric: "previous_touch_quantity:ask", Category: HardSupport},
	{Source: SourceToxicity, Metric: "touch_price_log_change:bid", Category: MechanicalCollapse},
	{Source: SourceToxicity, Metric: "touch_price_log_change:ask", Category: MechanicalCollapse},
	{Source: SourceToxicity, Metric: "unfilled_residual_quantity:ask", Category: MechanicalCollapse},
	{Source: SourceToxicity, Metric: "fill_fraction_divergence:bid", Category: ActiveReversal},
	{Source: SourceToxicity, Metric: "fill_fraction_divergence:ask", Category: ActiveReversal},
	{Source: SourceToxicity, Metric: "fill_fraction_baseline:bid", Category: ThermalExhaustion},
	{Source: SourceToxicity, Metric: "fill_fraction_baseline:ask", Category: ThermalExhaustion},
	{Source: SourceToxicity, Metric: "bracket_trade_quantity", Category: HiddenAbsorption},
	{Source: SourceToxicity, Metric: "touch_fill_fraction:ask", Category: HiddenAbsorption},
	{Source: SourceToxicity, Metric: "touch_fill_quantity:ask", Category: HiddenAbsorption},
	{Source: SourceToxicity, Metric: "touch_fill_rate:ask", Category: HiddenAbsorption},
	{Source: SourceToxicity, Metric: "touch_quantity:ask", Category: ExtremeScarcity},
	{Source: SourceToxicity, Metric: "touch_quantity:bid", Category: ExtremeScarcity},
	{Source: SourceToxicity, Metric: "best_price:ask", Category: Equilibrium},
	{Source: SourceToxicity, Metric: "best_price:bid", Category: Equilibrium},
	{Source: SourceToxicity, Metric: "previous_best_price:ask", Category: Equilibrium},
	{Source: SourceToxicity, Metric: "previous_best_price:bid", Category: Equilibrium},

	// Morphology: whole-book shape dislocation.
	{Source: SourceMorphology, Metric: "morphology_change", Category: Turbulent},
	{Source: SourceMorphology, Metric: "morphology_change_baseline", Category: Turbulent},
	{Source: SourceMorphology, Metric: "morphology_change", Category: BookThinning},
	{Source: SourceMorphology, Metric: "book_shape_distance", Category: BookThinning},
	{Source: SourceMorphology, Metric: "book_shape_ks", Category: BookThinning},
	{Source: SourceMorphology, Metric: "entropy:bid", Category: DenseNeutrality},
	{Source: SourceMorphology, Metric: "entropy:ask", Category: DenseNeutrality},
	{Source: SourceMorphology, Metric: "concentration:bid", Category: LoadedImbalance},
	{Source: SourceMorphology, Metric: "concentration:ask", Category: LoadedImbalance},

	// VerticalIgnition corroborated complex: PumpDump volume breakout anchors
	// it; Hawkes near-critical cascade clustering, DepthFlow ask-side mutation,
	// and CVD aggressive drive corroborate.
	{Source: SourcePumpDump, Metric: "volume_bar_quantity", Category: VerticalIgnition},
	{Source: SourceCVD, Metric: "signed_net_fraction_zscore", Category: VerticalIgnition},
	{Source: SourceHawkes, Metric: "branching_spectral_radius", Category: VerticalIgnition},
	{Source: SourceHawkes, Metric: "log_likelihood_gain_per_event_vs_poisson", Category: VerticalIgnition},
	{Source: SourceDepthFlow, Metric: "observed_notional_imbalance_zscore", Category: VerticalIgnition},
}


