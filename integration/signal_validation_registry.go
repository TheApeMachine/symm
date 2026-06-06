package integration

import (
	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
SignalCategoryProbe is one controlled market fixture and the exact category the
signal must publish for the probe symbol. Failures are expected until the synthetic
tape reliably produces that classification.
*/
type SignalCategoryProbe struct {
	Source    types.SourceType
	Category  types.CategoryType
	Symbol    string
	Condition string
	Fixture   SignalFixtureKey
	Scenario  func(probe SignalCategoryProbe) Scenario
}

/*
signalCategoryProbes is the full category surface area per measurement source
(mirrors each signal package registry). One integration scenario per row.
*/
func signalCategoryProbes() []SignalCategoryProbe {
	probes := make([]SignalCategoryProbe, 0, 48)
	probes = append(probes, cvdCategoryProbes()...)
	probes = append(probes, fluidCategoryProbes()...)
	probes = append(probes, hawkesCategoryProbes()...)
	probes = append(probes, depthflowCategoryProbes()...)
	probes = append(probes, sentimentCategoryProbes()...)
	probes = append(probes, liquidityCategoryProbes()...)
	probes = append(probes, pumpdumpCategoryProbes()...)
	probes = append(probes, exhaustCategoryProbes()...)
	probes = append(probes, causalCategoryProbes()...)
	probes = append(probes, leadlagCategoryProbes()...)
	probes = append(probes, correlationCategoryProbes()...)
	probes = append(probes, toxicityCategoryProbes()...)

	return probes
}

func cvdCategoryProbes() []SignalCategoryProbe {
	return []SignalCategoryProbe{
		probe(types.SourceCVD, types.CategoryAggressiveDrive, FixtureCVDAggressiveDrive,
			"sustained one-sided buy aggression on the tape"),
		probe(types.SourceCVD, types.CategoryHiddenAbsorption, FixtureCVDHiddenAbsorption,
			"high volume with muted price progress (absorption)"),
		probe(types.SourceCVD, types.CategoryStochasticBalance, FixtureCVDStochasticBalance,
			"balanced two-sided flow around the local mean"),
		probe(types.SourceCVD, types.CategoryVolumeStarvation, FixtureCVDVolumeStarvation,
			"negligible executed volume relative to history"),
	}
}

func fluidCategoryProbes() []SignalCategoryProbe {
	return []SignalCategoryProbe{
		probe(types.SourceFluid, types.CategoryLaminar, FixtureFluidLaminar,
			"calm book with low divergence and Reynolds number"),
		probe(types.SourceFluid, types.CategoryTurbulent, FixtureFluidTurbulent,
			"turbulence dominates divergence"),
		probe(types.SourceFluid, types.CategoryInertial, FixtureFluidInertial,
			"strong divergence with inertial Reynolds read"),
		probe(types.SourceFluid, types.CategoryViscous, FixtureFluidViscous,
			"high viscosity read below the viscous threshold"),
	}
}

func hawkesCategoryProbes() []SignalCategoryProbe {
	return []SignalCategoryProbe{
		probe(types.SourceHawkes, types.CategoryFrenzy, FixtureHawkesFrenzy,
			"asymmetric buy-side Hawkes intensity"),
		probe(types.SourceHawkes, types.CategorySaturation, FixtureHawkesSaturation,
			"near-critical spectral radius from clustered prints"),
		probe(types.SourceHawkes, types.CategoryOrganic, FixtureHawkesOrganic,
			"low clustering with organic baseline intensity"),
		probe(types.SourceHawkes, types.CategoryExhaustion, FixtureHawkesExhaustion,
			"intensity collapse after a burst"),
	}
}

func depthflowCategoryProbes() []SignalCategoryProbe {
	return []SignalCategoryProbe{
		probe(types.SourceDepthFlow, types.CategoryLoadedImbalance, FixtureDepthflowLoadedImbalance,
			"persistent bid-heavy book without spoof pathology"),
		probe(types.SourceDepthFlow, types.CategorySpoofTrap, FixtureDepthflowSpoofTrap,
			"pathological near-touch imbalance (spoof trap)"),
		probe(types.SourceDepthFlow, types.CategoryBookThinning, FixtureDepthflowBookThinning,
			"depth pulls away from the touch"),
		probe(types.SourceDepthFlow, types.CategoryDenseNeutrality, FixtureDepthflowDenseNeutrality,
			"flat, balanced book at the touch"),
	}
}

func sentimentCategoryProbes() []SignalCategoryProbe {
	return []SignalCategoryProbe{
		probe(types.SourceSentiment, types.CategorySystemicSlump, FixtureSentimentSystemicSlump,
			"weak cross-section breadth with negative changes"),
		probe(types.SourceSentiment, types.CategoryRiskOnSurge, FixtureSentimentRiskOnSurge,
			"broad positive breadth above the surge threshold"),
		probe(types.SourceSentiment, types.CategoryDivergentMove, FixtureSentimentDivergentMove,
			"leader symbol moves against the broad tape"),
	}
}

func liquidityCategoryProbes() []SignalCategoryProbe {
	return []SignalCategoryProbe{
		probe(types.SourceLiquidity, types.CategoryRobustLiquidity, FixtureLiquidityRobust,
			"primary symbol quote volume in the top peer quartile"),
		probe(types.SourceLiquidity, types.CategoryMedianDepth, FixtureLiquidityMedianDepth,
			"primary symbol quote volume between peer quartiles"),
		probe(types.SourceLiquidity, types.CategoryExtremeScarcity, FixtureLiquidityExtremeScarcity,
			"primary symbol quote volume in the bottom peer quartile"),
	}
}

func pumpdumpCategoryProbes() []SignalCategoryProbe {
	return []SignalCategoryProbe{
		probe(types.SourcePumpDump, types.CategoryVerticalIgnition, FixturePumpdumpVerticalIgnition,
			"accelerating buy tape with rising lift scores"),
		probe(types.SourcePumpDump, types.CategoryCoiledCompression, FixturePumpdumpCoiledCompression,
			"tight range with coiled volume precursor"),
		probe(types.SourcePumpDump, types.CategoryOrganicTrend, FixturePumpdumpOrganicTrend,
			"steady organic drift without ignition"),
		probe(types.SourcePumpDump, types.CategoryFadedExhaustion, FixturePumpdumpFadedExhaustion,
			"lift fades after an impulse"),
	}
}

func exhaustCategoryProbes() []SignalCategoryProbe {
	return []SignalCategoryProbe{
		probe(types.SourceExhaustion, types.CategoryMechanicalCollapse, FixtureExhaustMechanicalCollapse,
			"book thinning dominates exit score"),
		probe(types.SourceExhaustion, types.CategoryFragileExpansion, FixtureExhaustFragileExpansion,
			"spread widening dominates exit score"),
		probe(types.SourceExhaustion, types.CategoryThermalExhaustion, FixtureExhaustThermalExhaustion,
			"pressure fade dominates exit score"),
		probe(types.SourceExhaustion, types.CategoryActiveReversal, FixtureExhaustActiveReversal,
			"imbalance flip dominates exit score"),
	}
}

func causalCategoryProbes() []SignalCategoryProbe {
	return []SignalCategoryProbe{
		probe(types.SourceCausal, types.CategoryEndogenousAlpha, FixtureCausalEndogenousAlpha,
			"intervention ladder read with endogenous alpha"),
		probe(types.SourceCausal, types.CategorySystemicBeta, FixtureCausalSystemicBeta,
			"macro association drives beta read"),
		probe(types.SourceCausal, types.CategoryLiquidityShock, FixtureCausalLiquidityShock,
			"regime inversion shock on the ladder"),
		probeWithScenario(types.SourceCausal, types.CategoryCausalNoise, FixtureCausalCausalNoise,
			"buy pressure without price change (noise)", causalNoiseScenario),
	}
}

func leadlagCategoryProbes() []SignalCategoryProbe {
	return []SignalCategoryProbe{
		probeWithScenario(types.SourceLeadLag, types.CategoryAnchorStall, FixtureLeadlagAnchorStall,
			"flat anchor with follower drift", leadlagAnchorStallScenario),
		probeWithScenario(types.SourceLeadLag, types.CategoryInefficientLag, FixtureLeadlagInefficientLag,
			"follower lags anchor beyond min lag fraction", leadlagInefficientLagScenario),
		probeWithScenario(types.SourceLeadLag, types.CategorySynchronizedDrift, FixtureLeadlagSynchronizedDrift,
			"anchor and follower move together", leadlagSynchronizedDriftScenario),
		probeWithScenario(types.SourceLeadLag, types.CategoryDecoupledMove, FixtureLeadlagDecoupledMove,
			"low correlation between anchor and follower", leadlagDecoupledMoveScenario),
	}
}

func correlationCategoryProbes() []SignalCategoryProbe {
	return []SignalCategoryProbe{
		probeWithScenario(types.SourceCorrelation, types.CategorySystemicHerd, FixtureCorrelationSystemicHerd,
			"coordinated cross-section buy drift", correlationHerdScenario),
		probeWithScenario(types.SourceCorrelation, types.CategoryDecoupledAlpha, FixtureCorrelationDecoupledAlpha,
			"one symbol diverges from the herd fingerprint", correlationDecoupledScenario),
		probeWithScenario(types.SourceCorrelation, types.CategoryStochasticNoise, FixtureCorrelationStochasticNoise,
			"quiet uncorrelated movement", correlationNoiseScenario),
		probeWithScenario(types.SourceCorrelation, types.CategoryDivergentStress, FixtureCorrelationDivergentStress,
			"contrarian symbol against the majority fingerprint", correlationDivergentStressScenario),
	}
}

func toxicityCategoryProbes() []SignalCategoryProbe {
	return []SignalCategoryProbe{
		probe(types.SourceToxicity, types.CategoryToxicBluff, FixtureToxicityToxicBluff,
			"near-touch cancel-heavy book updates"),
		probe(types.SourceToxicity, types.CategoryLiquidityVacuum, FixtureToxicityLiquidityVacuum,
			"vacuum after liquidity pulls from the touch"),
		probe(types.SourceToxicity, types.CategoryHardSupport, FixtureToxicityHardSupport,
			"stable size at the touch without bluff cancels"),
	}
}

func probe(
	source types.SourceType,
	category types.CategoryType,
	fixture SignalFixtureKey,
	condition string,
) SignalCategoryProbe {
	return SignalCategoryProbe{
		Source:    source,
		Category:  category,
		Symbol:    testSymbolPrimary,
		Condition: condition,
		Fixture:   fixture,
	}
}

func probeWithScenario(
	source types.SourceType,
	category types.CategoryType,
	fixture SignalFixtureKey,
	condition string,
	extend func(SignalCategoryProbe) Scenario,
) SignalCategoryProbe {
	entry := probe(source, category, fixture, condition)
	entry.Scenario = extend

	return entry
}
