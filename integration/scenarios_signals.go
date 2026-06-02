package integration

import (
	"time"

	"github.com/theapemachine/symm/market/perspectives"
)

func signalScenarios() []Scenario {
	return []Scenario{
		{
			ID:   "signal.cvd_flow",
			Name: "CVD publishes executed-flow measurement from synthetic tape",
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendBaselineMarket()
			},
			SettleDelay: 600 * time.Millisecond,
			Checks: []ScenarioCheck{
				checkMeasurementSource("cvd.source", "CVD measurement observed", perspectives.SourceCVD, testSymbolPrimary),
				checkCategoryObserved("cvd.category", "CVD assigns a flow category", perspectives.SourceCVD,
					perspectives.CategoryAggressiveDrive,
					perspectives.CategoryHiddenAbsorption,
					perspectives.CategoryStochasticBalance,
					perspectives.CategoryVolumeStarvation,
				),
			},
		},
		{
			ID:   "signal.fluid_mechanics",
			Name: "Fluid publishes mechanical perspective from synthetic book",
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendBaselineMarket()
			},
			SettleDelay: 600 * time.Millisecond,
			Checks: []ScenarioCheck{
				checkMeasurementSource("fluid.source", "Fluid measurement observed", perspectives.SourceFluid, testSymbolPrimary),
				checkCategoryObserved("fluid.category", "Fluid assigns a mechanical category", perspectives.SourceFluid,
					perspectives.CategoryLaminar, perspectives.CategoryTurbulent,
					perspectives.CategoryInertial, perspectives.CategoryViscous,
				),
			},
		},
		{
			ID:   "signal.hawkes_burst",
			Name: "Hawkes fits a clustered trade burst",
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendInstrumentCatalog()
				builder.AppendTicker(testSymbolPrimary, 100, 99, 101, 0)
				builder.AppendTradeBurst(testSymbolPrimary, 128, 100, 1.5, "alternate")
			},
			SettleDelay: 800 * time.Millisecond,
			Checks: []ScenarioCheck{
				checkMeasurementSource("hawkes.source", "Hawkes measurement observed", perspectives.SourceHawkes, testSymbolPrimary),
				checkCategoryObserved("hawkes.category", "Hawkes assigns a thermal category", perspectives.SourceHawkes,
					perspectives.CategoryFrenzy, perspectives.CategorySaturation,
					perspectives.CategoryOrganic, perspectives.CategoryExhaustion,
				),
			},
		},
		{
			ID:   "signal.depthflow_imbalance",
			Name: "Depthflow publishes depth-pressure reading",
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendInstrumentCatalog()
				builder.AppendDepthflowTape(testSymbolPrimary)
			},
			SettleDelay: 600 * time.Millisecond,
			Checks: []ScenarioCheck{
				checkMeasurementSource("depthflow.source", "Depthflow measurement observed", perspectives.SourceDepthFlow, testSymbolPrimary),
				checkCategoryObserved("depthflow.category", "Depthflow assigns an imbalance category", perspectives.SourceDepthFlow,
					perspectives.CategoryLoadedImbalance, perspectives.CategorySpoofTrap,
					perspectives.CategoryBookThinning, perspectives.CategoryDenseNeutrality,
				),
			},
		},
		{
			ID:   "signal.sentiment_slump",
			Name: "Sentiment classifies systemic slump under weak breadth",
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendInstrumentCatalog()
				builder.AppendSentimentSlumpCrossSection()
			},
			SettleDelay: 600 * time.Millisecond,
			Checks: []ScenarioCheck{
				checkCategoryExact("sentiment.category", "Sentiment maps weak breadth to systemic slump",
					perspectives.SourceSentiment, testSymbolPrimary, perspectives.CategorySystemicSlump),
			},
		},
		{
			ID:   "signal.liquidity_robust",
			Name: "Liquidity ranks symbol versus peer quote volume",
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendInstrumentCatalog()
				builder.AppendLiquidityCrossSection()
			},
			SettleDelay: 600 * time.Millisecond,
			Checks: []ScenarioCheck{
				checkCategoryObserved("liquidity.category", "Liquidity assigns a scarcity category", perspectives.SourceLiquidity,
					perspectives.CategoryRobustLiquidity, perspectives.CategoryMedianDepth,
					perspectives.CategoryExtremeScarcity,
				),
			},
		},
		{
			ID:   "signal.pumpdump_lift",
			Name: "Pumpdump detects developing lift from rising buy tape",
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendInstrumentCatalog()
				builder.AppendTicker(testSymbolPrimary, 10, 9.9, 10.1, 2)
				builder.AppendPumpLift(testSymbolPrimary, 28)
			},
			SettleDelay: 700 * time.Millisecond,
			Checks: []ScenarioCheck{
				checkMeasurementSource("pumpdump.source", "Pumpdump measurement observed", perspectives.SourcePumpDump, testSymbolPrimary),
				checkCategoryObserved("pumpdump.category", "Pumpdump assigns a pump category", perspectives.SourcePumpDump,
					perspectives.CategoryVerticalIgnition, perspectives.CategoryCoiledCompression,
					perspectives.CategoryOrganicTrend, perspectives.CategoryFadedExhaustion,
				),
			},
		},
		{
			ID:   "signal.exhaust_thinning",
			Name: "Exhaust detects spread widening and depth thinning",
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendInstrumentCatalog()
				builder.AppendTicker(testSymbolPrimary, 100, 99, 101, 0)
				builder.AppendBookThinning(testSymbolPrimary, 8)
			},
			SettleDelay: 700 * time.Millisecond,
			Checks: []ScenarioCheck{
				checkMeasurementSource("exhaust.source", "Exhaust measurement observed", perspectives.SourceExhaustion, testSymbolPrimary),
				checkCategoryObserved("exhaust.category", "Exhaust assigns a decay category", perspectives.SourceExhaustion,
					perspectives.CategoryMechanicalCollapse, perspectives.CategoryThermalExhaustion,
					perspectives.CategoryFragileExpansion, perspectives.CategoryActiveReversal,
				),
			},
		},
		{
			ID:   "signal.causal_structure",
			Name: "Causal publishes structural reading across the cross-section",
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendCausalCrossSection()
			},
			SettleDelay: 1200 * time.Millisecond,
			Checks: []ScenarioCheck{
				checkMeasurementSource("causal.source", "Causal measurement observed", perspectives.SourceCausal, testSymbolPrimary),
				checkCategoryObserved("causal.category", "Causal assigns a structural category", perspectives.SourceCausal,
					perspectives.CategoryEndogenousAlpha, perspectives.CategorySystemicBeta,
					perspectives.CategoryLiquidityShock, perspectives.CategoryCausalNoise,
				),
			},
		},
		{
			ID:   "signal.leadlag_stall",
			Name: "Leadlag publishes anchor stall when anchor is flat",
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendInstrumentCatalog()
				builder.AppendLeadLagStall()
			},
			SettleDelay: 900 * time.Millisecond,
			Checks: []ScenarioCheck{
				checkCategoryObserved("leadlag.category", "Leadlag assigns anchor stall on flat anchor", perspectives.SourceLeadLag,
					perspectives.CategoryAnchorStall, perspectives.CategoryInefficientLag,
					perspectives.CategorySynchronizedDrift, perspectives.CategoryDecoupledMove,
				),
			},
		},
		{
			ID:   "signal.correlation_herd",
			Name: "Correlation classifies coordinated cross-section movement",
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendInstrumentCatalog()
				builder.AppendCorrelationHerd()
			},
			RunTimeout:  12 * time.Second,
			SettleDelay: 10 * time.Second,
			Checks: []ScenarioCheck{
				checkCategoryObserved("correlation.category", "Correlation assigns herd or stress category", perspectives.SourceCorrelation,
					perspectives.CategorySystemicHerd, perspectives.CategoryDecoupledAlpha,
					perspectives.CategoryStochasticNoise, perspectives.CategoryDivergentStress,
				),
			},
		},
		{
			ID:   "signal.toxicity_bluff",
			Name: "Toxicity flags near-touch cancel-heavy book updates",
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendInstrumentCatalog()
				builder.AppendToxicityCancelWall(testSymbolPrimary, 100)
			},
			SettleDelay: 700 * time.Millisecond,
			Checks: []ScenarioCheck{
				checkMeasurementSource("toxicity.source", "Toxicity measurement observed", perspectives.SourceToxicity, testSymbolPrimary),
				checkCategoryObserved("toxicity.category", "Toxicity assigns book-quality category", perspectives.SourceToxicity,
					perspectives.CategoryToxicBluff, perspectives.CategoryLiquidityVacuum,
					perspectives.CategoryHardSupport,
				),
			},
		},
	}
}
