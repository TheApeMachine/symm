package integration

import "github.com/theapemachine/symm/market/perspectives"

/*
SignalCategoryProbe is one controlled market fixture and the exact category the
signal must publish for the probe symbol. Failures are expected until the synthetic
tape reliably produces that classification.
*/
type SignalCategoryProbe struct {
	Source    perspectives.SourceType
	Category  perspectives.CategoryType
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
		probe(perspectives.SourceCVD, perspectives.CategoryAggressiveDrive, FixtureCVDAggressiveDrive,
			"sustained one-sided buy aggression on the tape"),
		probe(perspectives.SourceCVD, perspectives.CategoryHiddenAbsorption, FixtureCVDHiddenAbsorption,
			"high volume with muted price progress (absorption)"),
		probe(perspectives.SourceCVD, perspectives.CategoryStochasticBalance, FixtureCVDStochasticBalance,
			"balanced two-sided flow around the local mean"),
		probe(perspectives.SourceCVD, perspectives.CategoryVolumeStarvation, FixtureCVDVolumeStarvation,
			"negligible executed volume relative to history"),
	}
}

func fluidCategoryProbes() []SignalCategoryProbe {
	return []SignalCategoryProbe{
		probe(perspectives.SourceFluid, perspectives.CategoryLaminar, FixtureFluidLaminar,
			"calm book with low divergence and Reynolds number"),
		probe(perspectives.SourceFluid, perspectives.CategoryTurbulent, FixtureFluidTurbulent,
			"turbulence dominates divergence"),
		probe(perspectives.SourceFluid, perspectives.CategoryInertial, FixtureFluidInertial,
			"strong divergence with inertial Reynolds read"),
		probe(perspectives.SourceFluid, perspectives.CategoryViscous, FixtureFluidViscous,
			"high viscosity read below the viscous threshold"),
	}
}

func hawkesCategoryProbes() []SignalCategoryProbe {
	return []SignalCategoryProbe{
		probe(perspectives.SourceHawkes, perspectives.CategoryFrenzy, FixtureHawkesFrenzy,
			"asymmetric buy-side Hawkes intensity"),
		probe(perspectives.SourceHawkes, perspectives.CategorySaturation, FixtureHawkesSaturation,
			"near-critical spectral radius from clustered prints"),
		probe(perspectives.SourceHawkes, perspectives.CategoryOrganic, FixtureHawkesOrganic,
			"low clustering with organic baseline intensity"),
		probe(perspectives.SourceHawkes, perspectives.CategoryExhaustion, FixtureHawkesExhaustion,
			"intensity collapse after a burst"),
	}
}

func depthflowCategoryProbes() []SignalCategoryProbe {
	return []SignalCategoryProbe{
		probe(perspectives.SourceDepthFlow, perspectives.CategoryLoadedImbalance, FixtureDepthflowLoadedImbalance,
			"persistent bid-heavy book without spoof pathology"),
		probe(perspectives.SourceDepthFlow, perspectives.CategorySpoofTrap, FixtureDepthflowSpoofTrap,
			"pathological near-touch imbalance (spoof trap)"),
		probe(perspectives.SourceDepthFlow, perspectives.CategoryBookThinning, FixtureDepthflowBookThinning,
			"depth pulls away from the touch"),
		probe(perspectives.SourceDepthFlow, perspectives.CategoryDenseNeutrality, FixtureDepthflowDenseNeutrality,
			"flat, balanced book at the touch"),
	}
}

func sentimentCategoryProbes() []SignalCategoryProbe {
	return []SignalCategoryProbe{
		probe(perspectives.SourceSentiment, perspectives.CategorySystemicSlump, FixtureSentimentSystemicSlump,
			"weak cross-section breadth with negative changes"),
		probe(perspectives.SourceSentiment, perspectives.CategoryRiskOnSurge, FixtureSentimentRiskOnSurge,
			"broad positive breadth above the surge threshold"),
		probe(perspectives.SourceSentiment, perspectives.CategoryDivergentMove, FixtureSentimentDivergentMove,
			"leader symbol moves against the broad tape"),
	}
}

func liquidityCategoryProbes() []SignalCategoryProbe {
	return []SignalCategoryProbe{
		probe(perspectives.SourceLiquidity, perspectives.CategoryRobustLiquidity, FixtureLiquidityRobust,
			"primary symbol quote volume in the top peer quartile"),
		probe(perspectives.SourceLiquidity, perspectives.CategoryMedianDepth, FixtureLiquidityMedianDepth,
			"primary symbol quote volume between peer quartiles"),
		probe(perspectives.SourceLiquidity, perspectives.CategoryExtremeScarcity, FixtureLiquidityExtremeScarcity,
			"primary symbol quote volume in the bottom peer quartile"),
	}
}

func pumpdumpCategoryProbes() []SignalCategoryProbe {
	return []SignalCategoryProbe{
		probe(perspectives.SourcePumpDump, perspectives.CategoryVerticalIgnition, FixturePumpdumpVerticalIgnition,
			"accelerating buy tape with rising lift scores"),
		probe(perspectives.SourcePumpDump, perspectives.CategoryCoiledCompression, FixturePumpdumpCoiledCompression,
			"tight range with coiled volume precursor"),
		probe(perspectives.SourcePumpDump, perspectives.CategoryOrganicTrend, FixturePumpdumpOrganicTrend,
			"steady organic drift without ignition"),
		probe(perspectives.SourcePumpDump, perspectives.CategoryFadedExhaustion, FixturePumpdumpFadedExhaustion,
			"lift fades after an impulse"),
	}
}

func exhaustCategoryProbes() []SignalCategoryProbe {
	return []SignalCategoryProbe{
		probe(perspectives.SourceExhaustion, perspectives.CategoryMechanicalCollapse, FixtureExhaustMechanicalCollapse,
			"book thinning dominates exit score"),
		probe(perspectives.SourceExhaustion, perspectives.CategoryFragileExpansion, FixtureExhaustFragileExpansion,
			"spread widening dominates exit score"),
		probe(perspectives.SourceExhaustion, perspectives.CategoryThermalExhaustion, FixtureExhaustThermalExhaustion,
			"pressure fade dominates exit score"),
		probe(perspectives.SourceExhaustion, perspectives.CategoryActiveReversal, FixtureExhaustActiveReversal,
			"imbalance flip dominates exit score"),
	}
}

func causalCategoryProbes() []SignalCategoryProbe {
	return []SignalCategoryProbe{
		probe(perspectives.SourceCausal, perspectives.CategoryEndogenousAlpha, FixtureCausalEndogenousAlpha,
			"intervention ladder read with endogenous alpha"),
		probe(perspectives.SourceCausal, perspectives.CategorySystemicBeta, FixtureCausalSystemicBeta,
			"macro association drives beta read"),
		probe(perspectives.SourceCausal, perspectives.CategoryLiquidityShock, FixtureCausalLiquidityShock,
			"regime inversion shock on the ladder"),
		probeWithScenario(perspectives.SourceCausal, perspectives.CategoryCausalNoise, FixtureCausalCausalNoise,
			"buy pressure without price change (noise)", causalNoiseScenario),
	}
}

func leadlagCategoryProbes() []SignalCategoryProbe {
	return []SignalCategoryProbe{
		probeWithScenario(perspectives.SourceLeadLag, perspectives.CategoryAnchorStall, FixtureLeadlagAnchorStall,
			"flat anchor with follower drift", leadlagAnchorStallScenario),
		probeWithScenario(perspectives.SourceLeadLag, perspectives.CategoryInefficientLag, FixtureLeadlagInefficientLag,
			"follower lags anchor beyond min lag fraction", leadlagInefficientLagScenario),
		probeWithScenario(perspectives.SourceLeadLag, perspectives.CategorySynchronizedDrift, FixtureLeadlagSynchronizedDrift,
			"anchor and follower move together", leadlagSynchronizedDriftScenario),
		probeWithScenario(perspectives.SourceLeadLag, perspectives.CategoryDecoupledMove, FixtureLeadlagDecoupledMove,
			"low correlation between anchor and follower", leadlagDecoupledMoveScenario),
	}
}

func correlationCategoryProbes() []SignalCategoryProbe {
	return []SignalCategoryProbe{
		probeWithScenario(perspectives.SourceCorrelation, perspectives.CategorySystemicHerd, FixtureCorrelationSystemicHerd,
			"coordinated cross-section buy drift", correlationHerdScenario),
		probeWithScenario(perspectives.SourceCorrelation, perspectives.CategoryDecoupledAlpha, FixtureCorrelationDecoupledAlpha,
			"one symbol diverges from the herd fingerprint", correlationDecoupledScenario),
		probeWithScenario(perspectives.SourceCorrelation, perspectives.CategoryStochasticNoise, FixtureCorrelationStochasticNoise,
			"quiet uncorrelated movement", correlationNoiseScenario),
		probeWithScenario(perspectives.SourceCorrelation, perspectives.CategoryDivergentStress, FixtureCorrelationDivergentStress,
			"contrarian symbol against the majority fingerprint", correlationDivergentStressScenario),
	}
}

func toxicityCategoryProbes() []SignalCategoryProbe {
	return []SignalCategoryProbe{
		probe(perspectives.SourceToxicity, perspectives.CategoryToxicBluff, FixtureToxicityToxicBluff,
			"near-touch cancel-heavy book updates"),
		probe(perspectives.SourceToxicity, perspectives.CategoryLiquidityVacuum, FixtureToxicityLiquidityVacuum,
			"vacuum after liquidity pulls from the touch"),
		probe(perspectives.SourceToxicity, perspectives.CategoryHardSupport, FixtureToxicityHardSupport,
			"stable size at the touch without bluff cancels"),
	}
}

func probe(
	source perspectives.SourceType,
	category perspectives.CategoryType,
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
	source perspectives.SourceType,
	category perspectives.CategoryType,
	fixture SignalFixtureKey,
	condition string,
	extend func(SignalCategoryProbe) Scenario,
) SignalCategoryProbe {
	entry := probe(source, category, fixture, condition)
	entry.Scenario = extend

	return entry
}
