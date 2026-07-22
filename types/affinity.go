package types

import "slices"

/*
MetricAffinity names the categories one measured quantity is evidence for and
against. Supporting metrics justify a Supports edge to the category node;
Opposing metrics justify a Contradicts edge. Magnitude and baseline metrics
(strength, value, event counts) carry no category affinity and are omitted.
*/
type MetricAffinity struct {
	Supports []CategoryType
	Opposes  []CategoryType
}

/*
Lists reports whether this affinity names category as supporting or opposing
evidence, which defines whether its metric is missing from a category proof.
*/
func (affinity MetricAffinity) Lists(category CategoryType) bool {
	if slices.Contains(affinity.Supports, category) {
		return true
	}

	return slices.Contains(affinity.Opposes, category)
}

/*
CategoryAffinity maps each signal-emitted MetricType onto the categories it
supports or opposes. It is the cross-observable structure the evidence graph is
built from: a category is a hypothesis, and every measurement listed here is
typed evidence for or against it. Derived from each signal's measurements()
output; magnitude-only metrics are intentionally absent.
*/
var CategoryAffinity = map[MetricType]MetricAffinity{
	// depthflow (touch-level book imbalance)
	MetricLoadedScore: {
		Supports: []CategoryType{LoadedImbalance},
		Opposes:  []CategoryType{DenseNeutrality},
	},
	MetricSpoofScore: {
		Supports: []CategoryType{SpoofTrap, ToxicBluff},
		Opposes:  []CategoryType{HardSupport},
	},
	MetricThinScore: {
		Supports: []CategoryType{BookThinning, LiquidityVacuum},
		Opposes:  []CategoryType{RobustLiquidity},
	},
	MetricNeutralScore: {
		Supports: []CategoryType{DenseNeutrality, StochasticBalance},
		Opposes:  []CategoryType{LoadedImbalance, SpoofTrap},
	},

	// cvd (signed aggressor flow)
	MetricAbsorption: {
		Supports: []CategoryType{HiddenAbsorption},
		Opposes:  []CategoryType{AggressiveDrive},
	},
	MetricDrive: {
		Supports: []CategoryType{AggressiveDrive, RiskOnSurge},
		Opposes:  []CategoryType{HiddenAbsorption},
	},
	MetricBalance: {
		Supports: []CategoryType{StochasticBalance, Equilibrium},
		Opposes:  []CategoryType{AggressiveDrive},
	},
	MetricStarvation: {
		Supports: []CategoryType{VolumeStarvation, ExtremeScarcity},
		Opposes:  []CategoryType{RiskOnSurge},
	},

	// exhaust (microstructure decay)
	MetricMechanical: {
		Supports: []CategoryType{MechanicalCollapse},
		Opposes:  []CategoryType{OrganicTrend},
	},
	MetricThermal: {
		Supports: []CategoryType{ThermalExhaustion, Exhaustion},
		Opposes:  []CategoryType{Organic},
	},
	MetricFragile: {
		Supports: []CategoryType{FragileExpansion},
		Opposes:  []CategoryType{RobustLiquidity, HardSupport},
	},
	MetricReversal: {
		Supports: []CategoryType{ActiveReversal, FadedExhaustion},
		Opposes:  []CategoryType{OrganicTrend},
	},
	MetricUrgency: {
		Supports: []CategoryType{Frenzy, VerticalIgnition},
		Opposes:  []CategoryType{Laminar},
	},

	// fluid (mechanical order-book dynamics)
	MetricLaminarScore: {
		Supports: []CategoryType{Laminar, LaminarResonance},
		Opposes:  []CategoryType{Turbulent},
	},
	MetricTurbulentScore: {
		Supports: []CategoryType{Turbulent, TurbulentResonance},
		Opposes:  []CategoryType{Laminar},
	},
	MetricInertialScore: {
		Supports: []CategoryType{Inertial},
		Opposes:  []CategoryType{Viscous},
	},
	MetricViscousScore: {
		Supports: []CategoryType{Viscous},
		Opposes:  []CategoryType{Inertial},
	},
	MetricTurbulence: {
		Supports: []CategoryType{Turbulent, TurbulentResonance},
		Opposes:  []CategoryType{Laminar},
	},

	// hawkes (self-exciting arrivals)
	MetricSpectralRadius: {
		Supports: []CategoryType{Frenzy, Saturation, Turbulent},
		Opposes:  []CategoryType{StochasticBalance},
	},
	MetricExcitationAmplitude: {
		Supports: []CategoryType{Frenzy, AggressiveDrive},
		Opposes:  []CategoryType{StochasticBalance},
	},
	MetricConditionalIntensity: {
		Supports: []CategoryType{RiskOnSurge, Frenzy},
		Opposes:  []CategoryType{VolumeStarvation},
	},
	MetricBaselineIntensity: {
		Supports: []CategoryType{Organic, StochasticBalance},
		Opposes:  []CategoryType{Frenzy},
	},
	MetricImmediateOffspring: {
		Supports: []CategoryType{Saturation, Frenzy},
		Opposes:  []CategoryType{StochasticNoise},
	},
	MetricTotalDescendants: {
		Supports: []CategoryType{Saturation, Frenzy},
		Opposes:  []CategoryType{StochasticNoise},
	},
	MetricHawkesPoissonDelta: {
		Supports: []CategoryType{EndogenousAlpha, PhysicalField},
		Opposes:  []CategoryType{StochasticNoise, CausalNoise},
	},
	MetricCrossSelfDelta: {
		Supports: []CategoryType{SystemicBeta, SynchronizedDrift},
		Opposes:  []CategoryType{EndogenousAlpha},
	},

	// leadlag (cohort lead-lag relation)
	MetricInefficient: {
		Supports: []CategoryType{InefficientLag, DecoupledAlpha},
		Opposes:  []CategoryType{SynchronizedDrift},
	},
	MetricSync: {
		Supports: []CategoryType{SynchronizedDrift, SystemicHerd},
		Opposes:  []CategoryType{DecoupledMove},
	},
	MetricDecoupled: {
		Supports: []CategoryType{DecoupledMove, DecoupledAlpha, DivergentMove},
		Opposes:  []CategoryType{SynchronizedDrift},
	},
	MetricStall: {
		Supports: []CategoryType{AnchorStall},
		Opposes:  []CategoryType{OrganicTrend},
	},
	MetricSignedContempCorrelation: {
		Supports: []CategoryType{SynchronizedDrift},
		Opposes:  []CategoryType{DecoupledMove},
	},

	// correlation (cohort energy relation)
	MetricHerdScore: {
		Supports: []CategoryType{SystemicHerd, SynchronizedDrift, SystemicBeta},
		Opposes:  []CategoryType{DecoupledAlpha},
	},
	MetricAlphaScore: {
		Supports: []CategoryType{DecoupledAlpha, EndogenousAlpha},
		Opposes:  []CategoryType{SystemicHerd},
	},
	MetricNoiseScore: {
		Supports: []CategoryType{StochasticNoise, CausalNoise},
		Opposes:  []CategoryType{ForecastEdge},
	},
	MetricStressScore: {
		Supports: []CategoryType{DivergentStress, SystemicSlump},
		Opposes:  []CategoryType{Equilibrium},
	},
	MetricPeakScore: {
		Supports: []CategoryType{RiskOnSurge, LaminarResonance, TurbulentResonance},
	},
	MetricRelativeEnergy: {
		Supports: []CategoryType{RiskOnSurge, SystemicBeta},
	},

	// sentiment (breadth and leadership)
	MetricSurgeScore: {
		Supports: []CategoryType{RiskOnSurge},
		Opposes:  []CategoryType{SystemicSlump},
	},
	MetricSlumpScore: {
		Supports: []CategoryType{SystemicSlump, MechanicalCollapse},
		Opposes:  []CategoryType{RiskOnSurge},
	},
	MetricDivergentScore: {
		Supports: []CategoryType{DivergentMove, DivergentStress, DecoupledAlpha},
		Opposes:  []CategoryType{SynchronizedDrift},
	},

	// liquidity (turnover and executable touch-depth scarcity)
	MetricScarcityScore: {
		Supports: []CategoryType{ExtremeScarcity, VolumeStarvation, LiquidityVacuum},
		Opposes:  []CategoryType{RobustLiquidity},
	},
	MetricExecutableTouchDepth: {
		Supports: []CategoryType{MedianDepth, RobustLiquidity},
		Opposes:  []CategoryType{LiquidityVacuum},
	},

	// toxicity (level3 touch liquidity honesty)
	MetricRetreatingQuantity: {
		Supports: []CategoryType{ToxicBluff, SpoofTrap},
		Opposes:  []CategoryType{HardSupport},
	},
	MetricCancelledQuantity: {
		Supports: []CategoryType{ToxicBluff, SpoofTrap, BookThinning},
		Opposes:  []CategoryType{HardSupport},
	},
	MetricFillVolume: {
		Supports: []CategoryType{HardSupport, RobustLiquidity},
		Opposes:  []CategoryType{ToxicBluff, SpoofTrap},
	},
	MetricTouchQuantity: {
		Supports: []CategoryType{HardSupport, MedianDepth},
		Opposes:  []CategoryType{LiquidityVacuum},
	},

	// pumpdump (ignition and exhaustion)
	MetricIgnition: {
		Supports: []CategoryType{VerticalIgnition, RiskOnSurge},
		Opposes:  []CategoryType{AnchorStall},
	},
	MetricCompression: {
		Supports: []CategoryType{CoiledCompression},
		Opposes:  []CategoryType{LiquidityVacuum},
	},
	MetricExhaustion: {
		Supports: []CategoryType{FadedExhaustion, Exhaustion, ThermalExhaustion},
		Opposes:  []CategoryType{OrganicTrend},
	},
	MetricTrend: {
		Supports: []CategoryType{OrganicTrend},
		Opposes:  []CategoryType{FadedExhaustion},
	},
	MetricRVOL: {
		Supports: []CategoryType{Frenzy, RiskOnSurge},
		Opposes:  []CategoryType{VolumeStarvation},
	},
	MetricSpread: {
		Supports: []CategoryType{LiquidityVacuum, BookThinning},
		Opposes:  []CategoryType{RobustLiquidity},
	},
	MetricPrecursor: {
		Supports: []CategoryType{VerticalIgnition, CoiledCompression},
	},
}

/*
AffinityFor returns the category affinity for one metric and whether the metric
carries any. Magnitude-only metrics return a zero affinity and false.
*/
func AffinityFor(metric MetricType) (MetricAffinity, bool) {
	affinity, ok := CategoryAffinity[metric]
	return affinity, ok
}
