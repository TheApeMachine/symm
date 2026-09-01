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

func CategoryByIndex(index int) CategoryType {
	if index <= 0 || index > len(CategoryOrder) {
		return CategoryTypeNone
	}

	return CategoryOrder[index-1]
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
	{Source: SourceCorrelation, Metric: "relative_return_energy_zscore", Category: DecoupledAlpha},
	{Source: SourceCorrelation, Metric: "relative_return_energy_divergence", Category: DecoupledAlpha},
	{Source: SourceCorrelation, Metric: "correlation_zscore", Category: StochasticNoise},
	{Source: SourceCorrelation, Metric: "correlation_divergence", Category: DivergentStress},
	{Source: SourceCorrelation, Metric: "cohort_correlation_dispersion", Category: DivergentStress},

	// Lead-lag: temporal leadership / alignment between pairs.
	{Source: SourceLeadLag, Metric: "correlation_gain_zscore", Category: InefficientLag},
	{Source: SourceLeadLag, Metric: "contemporaneous_correlation", Category: SynchronizedDrift},
	{Source: SourceLeadLag, Metric: "lag_zscore", Category: DecoupledMove},
	{Source: SourceLeadLag, Metric: "best_lag_correlation_zscore", Category: AnchorStall},

	// CVD / executed flow: aggressive execution economics and midpoint response.
	{Source: SourceCVD, Metric: "signed_net_fraction_zscore", Category: AggressiveDrive},
	{Source: SourceCVD, Metric: "signed_net_fraction_divergence", Category: AggressiveDrive},
	{Source: SourceCVD, Metric: "gross_notional_rate_divergence", Category: VolumeStarvation},
	{Source: SourceCVD, Metric: "midpoint_return_rate_zscore", Category: StochasticBalance},
	{Source: SourceCVD, Metric: "flow_aligned_midpoint_return", Category: StochasticBalance},

	// Hawkes: arrival/excitation dynamics.
	{Source: SourceHawkes, Metric: "branching_spectral_radius", Category: Turbulent},
	{Source: SourceHawkes, Metric: "excitation_intensity:buy", Category: Frenzy},
	{Source: SourceHawkes, Metric: "excitation_intensity:sell", Category: Frenzy},
	{Source: SourceHawkes, Metric: "arrival_rate", Category: Inertial},

	// DepthFlow: facts present in each Level-3 mutation.
	{Source: SourceDepthFlow, Metric: "observed_notional_imbalance_zscore", Category: LoadedImbalance},
	{Source: SourceDepthFlow, Metric: "observed_notional_imbalance_divergence", Category: LoadedImbalance},
	{Source: SourceDepthFlow, Metric: "observed_notional_rate_zscore", Category: DenseNeutrality},
	{Source: SourceDepthFlow, Metric: "mutation_activity_imbalance", Category: BookThinning},

	// PumpDump: volume-clock ignition, spread compression, and midpoint return.
	{Source: SourcePumpDump, Metric: "volume_bar_quantity", Category: VerticalIgnition},
	{Source: SourcePumpDump, Metric: "volume_rate", Category: VerticalIgnition},
	{Source: SourcePumpDump, Metric: "volume_bar_quantity", Category: CoiledCompression},
	{Source: SourcePumpDump, Metric: "relative_spread", Category: CoiledCompression},
	{Source: SourcePumpDump, Metric: "spread_zscore", Category: CoiledCompression},
	{Source: SourcePumpDump, Metric: "trade_interval_seconds", Category: OrganicTrend},
	{Source: SourcePumpDump, Metric: "midpoint_return_zscore", Category: OrganicTrend},
	{Source: SourcePumpDump, Metric: "midpoint_return_zscore", Category: FadedExhaustion},
	{Source: SourcePumpDump, Metric: "notional_rate_zscore", Category: VerticalIgnition},

	// Liquidity: displayed executable capacity and spread.
	{Source: SourceLiquidity, Metric: "relative_spread", Category: ExtremeScarcity},
	{Source: SourceLiquidity, Metric: "two_sided_touch_notional", Category: RobustLiquidity},

	// Exhaustion metrics are REDUNDANT_WITH canonical Liquidity/Depthflow/CVD
	// sources (see signal/METRIC_MAP.md §8). Their Category consumers migrate
	// to canonical sources before the exhaust signal is removed; no new
	// SourceExhaustion evidence leg may be added here.

	// Sentiment: cross-sectional return state.
	{Source: SourceSentiment, Metric: "directional_consensus", Category: RiskOnSurge},
	{Source: SourceSentiment, Metric: "directional_agreement", Category: RiskOnSurge},
	{Source: SourceSentiment, Metric: "median_absolute_return_zscore", Category: DivergentMove},
	{Source: SourceSentiment, Metric: "return_dispersion_zscore", Category: DivergentMove},
	{Source: SourceSentiment, Metric: "breadth_zscore", Category: SystemicSlump},

	// Derivatives: leverage, liquidation, and basis.
	{Source: SourceDerivatives, Metric: "open_interest_growth_zscore", Category: LeveragedIgnition},
	{Source: SourceDerivatives, Metric: "open_interest_growth_rate", Category: LeveragedIgnition},
	{Source: SourceDerivatives, Metric: "liquidation_notional_rate", Category: AdverseLeverageBuildup},
	{Source: SourceDerivatives, Metric: "net_liquidation_notional", Category: LongDeleveraging},
	{Source: SourceDerivatives, Metric: "liquidation_share", Category: LongDeleveraging},
	{Source: SourceDerivatives, Metric: "basis_zscore", Category: DerivativesDecoupling},
	{Source: SourceDerivatives, Metric: "basis_rate", Category: DerivativesDecoupling},

	// Toxicity: liquidity disposition.
	{Source: SourceToxicity, Metric: "fill_fraction_zscore:bid", Category: LiquidityVacuum},
	{Source: SourceToxicity, Metric: "fill_fraction_zscore:ask", Category: LiquidityVacuum},

	// VerticalIgnition corroborated complex: PumpDump volume breakout anchors
	// it; Hawkes near-critical cascade clustering, DepthFlow ask-side mutation,
	// and CVD aggressive drive corroborate.
	{Source: SourcePumpDump, Metric: "volume_bar_quantity", Category: VerticalIgnition},
	{Source: SourceCVD, Metric: "signed_net_fraction_zscore", Category: VerticalIgnition},
	{Source: SourceHawkes, Metric: "branching_spectral_radius", Category: VerticalIgnition},
	{Source: SourceDepthFlow, Metric: "observed_notional_imbalance_zscore", Category: VerticalIgnition},
}
