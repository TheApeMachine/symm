package engine

/*
Category is a signal's self-classified verdict: exactly one row of that signal's
"perspective table". Each signal converts its raw microstructural metrics into
one of these labels and ships it on the measurement, so the decision layer
reasons over discrete, named market states instead of an opaque confidence
scalar. The empty Category ("") means the signal produced a reading but did not
commit to a labelled state (treated as no verdict by the decision layer).

Categories are grouped by the signal that emits them; CategorySignal maps any
category back to its owning source so the decision tree can ask "what did the
Hawkes perspective say?" without threading source strings around.
*/
type Category string

const (
	CategoryNone Category = ""

	// Fluid — the mechanical perspective (book "vapour pipe").
	CatLaminar   Category = "laminar"   // tight spread, low activity: smooth/consistent
	CatTurbulent Category = "turbulent" // turbulence/vorticity dominant: shattered/fragile
	CatInertial  Category = "inertial"  // Reynolds/divergence dominant: direct/heavy push
	CatViscous   Category = "viscous"   // wide spread, grinding at a wall: resistant

	// Hawkes — the thermal perspective (trade-cluster excitation).
	CatFrenzy     Category = "frenzy"     // moderate spectral radius, high asymmetry: directional
	CatSaturation Category = "saturation" // spectral radius -> 1.0, two-sided: contested/unstable
	CatOrganic    Category = "organic"    // low spectral radius near mu: healthy/quiet
	CatExhaustion Category = "exhaustion" // intensity below mu: stalled/dying

	// CVD — the absorption perspective (executed-volume truth).
	CatHiddenAbsorption  Category = "hidden_absorption"  // high net vol, flat price: iceberg
	CatAggressiveDrive   Category = "aggressive_drive"   // high net vol, price moving: trend support
	CatStochasticBalance Category = "stochastic_balance" // low net vol: equilibrium/choppy
	CatVolumeStarvation  Category = "volume_starvation"  // very low volume: dying interest

	// DepthFlow — the "weight of the book" perspective.
	CatLoadedImbalance Category = "loaded_imbalance" // high WBI, trade pressure agrees: gravity
	CatSpoofTrap       Category = "spoof_trap"       // high WBI, trade pressure contradicts: fake
	CatBookThinning    Category = "book_thinning"    // imbalance falling fast: crumbling
	CatDenseNeutrality Category = "dense_neutrality" // balanced, low pressure: robust/stable

	// LeadLag — the anchor perspective.
	CatInefficientLag    Category = "inefficient_lag"    // high corr, high lag fraction: catch-up
	CatSynchronizedDrift Category = "synchronized_drift" // high corr, low lag: systemic beta
	CatDecoupledMove     Category = "decoupled_move"     // low corr: idiosyncratic alpha
	CatAnchorStall       Category = "anchor_stall"       // anchor not moving: leadership exhausted

	// PumpDump — the ignition perspective.
	CatVerticalIgnition  Category = "vertical_ignition"  // high volume spike + precursor: launching
	CatCoiledCompression Category = "coiled_compression" // moderate lift, low move: pre-pump/loaded
	CatOrganicTrend      Category = "organic_trend"      // steady volume, moderate move: healthy
	CatFadedExhaustion   Category = "faded_exhaustion"   // volume falling, flat: leg is dead

	// Liquidity (basis) — the scarcity perspective.
	CatExtremeScarcity Category = "extreme_scarcity" // peak illiquidity: high convexity/fragile
	CatMedianDepth     Category = "median_depth"     // middle of pack: standard efficiency
	CatRobustLiquidity Category = "robust_liquidity" // deep: efficient/safe

	// Sentiment — the bullish-breadth perspective.
	CatRiskOnSurge   Category = "risk_on_surge"  // breadth > 0.55, strong leaders: rising tide
	CatDivergentMove Category = "divergent_move" // low breadth, strong single mover: idiosyncratic
	CatSystemicSlump Category = "systemic_slump" // low breadth, weak: global risk-off

	// Toxicity (bookflow) — the quality perspective.
	CatLiquidityVacuum Category = "liquidity_vacuum" // one side retreating: vacuum surcharge
	CatToxicBluff      Category = "toxic_bluff"      // near-touch cancels: manipulated/fake
	CatHardSupport     Category = "hard_support"     // high fill ratio: robust/sincere

	// Correlation — the herd-behaviour perspective.
	CatSystemicHerd    Category = "systemic_herd"    // corr > 0.85: global beta/momentum drift
	CatDecoupledAlpha  Category = "decoupled_alpha"  // low corr, high variance: unique driver
	CatStochasticNoise Category = "stochastic_noise" // low corr, low variance: quiet/indecisive
	CatDivergentStress Category = "divergent_stress" // negative corr: contrarian/relative weakness

	// Causal — the structural-origin perspective.
	CatEndogenousAlpha Category = "endogenous_alpha" // local flow is the driver: authentic
	CatSystemicBeta    Category = "systemic_beta"    // macro is the driver: passenger
	CatLiquidityShock  Category = "liquidity_shock"  // panic regime, liquidity void: fragile
	CatCausalNoise     Category = "causal_noise"     // no clear driver: stochastic equilibrium

	// Exhaust — the exit-thesis perspective (decay).
	CatMechanicalCollapse Category = "mechanical_collapse" // book thinning: crumbling walls
	CatThermalExhaustion  Category = "thermal_exhaustion"  // pressure fade: dying momentum
	CatFragileExpansion   Category = "fragile_expansion"   // spread widen: rising friction
	CatActiveReversal     Category = "active_reversal"     // imbalance flip: counter-attack
)

/*
CategorySignal maps a category to the source that owns it (matching the Source
field signals stamp on their measurements). Returns "" for CategoryNone or an
unknown category.
*/
func CategorySignal(category Category) string {
	switch category {
	case CatLaminar, CatTurbulent, CatInertial, CatViscous:
		return "fluid"
	case CatFrenzy, CatSaturation, CatOrganic, CatExhaustion:
		return "hawkes"
	case CatHiddenAbsorption, CatAggressiveDrive, CatStochasticBalance, CatVolumeStarvation:
		return "cvd"
	case CatLoadedImbalance, CatSpoofTrap, CatBookThinning, CatDenseNeutrality:
		return "depthflow"
	case CatInefficientLag, CatSynchronizedDrift, CatDecoupledMove, CatAnchorStall:
		return "leadlag"
	case CatVerticalIgnition, CatCoiledCompression, CatOrganicTrend, CatFadedExhaustion:
		return "pumpdump"
	case CatExtremeScarcity, CatMedianDepth, CatRobustLiquidity:
		return "liquidity"
	case CatRiskOnSurge, CatDivergentMove, CatSystemicSlump:
		return "sentiment"
	case CatLiquidityVacuum, CatToxicBluff, CatHardSupport:
		return "bookflow"
	case CatSystemicHerd, CatDecoupledAlpha, CatStochasticNoise, CatDivergentStress:
		return "correlation"
	case CatEndogenousAlpha, CatSystemicBeta, CatLiquidityShock, CatCausalNoise:
		return "causal"
	case CatMechanicalCollapse, CatThermalExhaustion, CatFragileExpansion, CatActiveReversal:
		return "exhaust"
	default:
		return ""
	}
}
