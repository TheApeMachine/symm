package integration

import (
	"time"

	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
)

// buildMeasurements and evaluateScenario share timeline construction for tree tests.

const testSymbol = "BTC/USD"

type treeScenario struct {
	name       string
	held       bool
	timeline   []measSpec
	wantAction logic.ActionType
	wantSide   trading.Side
}

type measSpec struct {
	source     logic.SourceType
	category   logic.CategoryType
	confidence float64
	surprise   float64
}

func allTreeScenarios() []treeScenario {
	return []treeScenario{
		{
			name: "exit_mechanical_collapse",
			held: true,
			timeline: []measSpec{{
				source:     logic.SourceExhaustion,
				category:   logic.CategoryMechanicalCollapse,
				confidence: 0.6,
				surprise:   1.2,
			}},
			wantAction: logic.ActionSettlePosition,
			wantSide:   trading.Sell,
		},
		{
			name: "exit_active_reversal",
			held: true,
			timeline: []measSpec{{
				source:     logic.SourceExhaustion,
				category:   logic.CategoryActiveReversal,
				confidence: 0.6,
				surprise:   1.2,
			}},
			wantAction: logic.ActionSettlePosition,
			wantSide:   trading.Sell,
		},
		{
			name: "exit_hawkes_saturation",
			held: true,
			timeline: []measSpec{{
				source:     logic.SourceHawkes,
				category:   logic.CategorySaturation,
				confidence: 0.6,
				surprise:   1.2,
			}},
			wantAction: logic.ActionSettlePosition,
			wantSide:   trading.Sell,
		},
		{
			name: "exit_fluid_turbulent",
			held: true,
			timeline: []measSpec{{
				source:     logic.SourceFluid,
				category:   logic.CategoryTurbulent,
				confidence: 0.6,
				surprise:   1.2,
			}},
			wantAction: logic.ActionSettlePosition,
			wantSide:   trading.Sell,
		},
		{
			name: "exit_causal_liquidity_shock",
			held: true,
			timeline: []measSpec{{
				source:     logic.SourceCausal,
				category:   logic.CategoryLiquidityShock,
				confidence: 0.6,
				surprise:   1.2,
			}},
			wantAction: logic.ActionSettlePosition,
			wantSide:   trading.Sell,
		},
		{
			name: "exit_manifold_liquidity_shock",
			held: true,
			timeline: []measSpec{{
				source:     logic.SourceManifold,
				category:   logic.CategoryLiquidityShock,
				confidence: 0.6,
				surprise:   1.2,
			}},
			wantAction: logic.ActionSettlePosition,
			wantSide:   trading.Sell,
		},
		{
			name: "exit_thermal_exhaustion",
			held: true,
			timeline: []measSpec{{
				source:     logic.SourceExhaustion,
				category:   logic.CategoryThermalExhaustion,
				confidence: 0.6,
				surprise:   1.2,
			}},
			wantAction: logic.ActionTakeProfit,
			wantSide:   trading.Sell,
		},
		{
			name: "exit_faded_exhaustion",
			held: true,
			timeline: []measSpec{{
				source:     logic.SourcePumpDump,
				category:   logic.CategoryFadedExhaustion,
				confidence: 0.6,
				surprise:   1.2,
			}},
			wantAction: logic.ActionTakeProfit,
			wantSide:   trading.Sell,
		},
		{
			name: "exit_systemic_beta",
			held: true,
			timeline: []measSpec{{
				source:     logic.SourceCausal,
				category:   logic.CategorySystemicBeta,
				confidence: 0.6,
				surprise:   1.2,
			}},
			wantAction: logic.ActionTakeProfit,
			wantSide:   trading.Sell,
		},
		{
			name: "entry_ignition",
			held: false,
			timeline: []measSpec{
				{logic.SourcePumpDump, logic.CategoryOrganicTrend, 0.6, 1.2},
				{logic.SourcePumpDump, logic.CategoryCoiledCompression, 0.6, 1.2},
				{logic.SourcePumpDump, logic.CategoryVerticalIgnition, 0.6, 1.2},
				{logic.SourceHawkes, logic.CategoryFrenzy, 0.6, 1.2},
				{logic.SourceCausal, logic.CategoryEndogenousAlpha, 0.6, 1.2},
				{logic.SourceCVD, logic.CategoryAggressiveDrive, 0.6, 1.2},
				{logic.SourceFluid, logic.CategoryLaminar, 0.6, 1.2},
			},
			wantAction: logic.ActionMarket,
			wantSide:   trading.Buy,
		},
		{
			name: "entry_catch_up",
			held: false,
			timeline: []measSpec{
				{logic.SourceLeadLag, logic.CategoryInefficientLag, 0.6, 1.2},
				{logic.SourceCausal, logic.CategoryEndogenousAlpha, 0.6, 1.2},
				{logic.SourceSentiment, logic.CategoryRiskOnSurge, 0.6, 1.2},
				{logic.SourceCVD, logic.CategoryAggressiveDrive, 0.6, 1.2},
				{logic.SourceHawkes, logic.CategoryFrenzy, 0.6, 1.2},
			},
			wantAction: logic.ActionMarket,
			wantSide:   trading.Buy,
		},
		{
			name: "entry_organic_trend",
			held: false,
			timeline: []measSpec{
				{logic.SourcePumpDump, logic.CategoryOrganicTrend, 0.6, 1.2},
				{logic.SourceCVD, logic.CategoryAggressiveDrive, 0.6, 1.2},
				{logic.SourceFluid, logic.CategoryLaminar, 0.6, 1.2},
				{logic.SourceSentiment, logic.CategoryRiskOnSurge, 0.6, 1.2},
			},
			wantAction: logic.ActionMarket,
			wantSide:   trading.Buy,
		},
		{
			name: "entry_absorption_breakout",
			held: false,
			timeline: []measSpec{
				{logic.SourceCVD, logic.CategoryHiddenAbsorption, 0.6, 1.2},
				{logic.SourceDepthFlow, logic.CategoryLoadedImbalance, 0.6, 1.2},
				{logic.SourcePumpDump, logic.CategoryOrganicTrend, 0.6, 1.2},
				{logic.SourcePumpDump, logic.CategoryCoiledCompression, 0.6, 1.2},
				{logic.SourcePumpDump, logic.CategoryVerticalIgnition, 0.6, 1.2},
				{logic.SourceFluid, logic.CategoryInertial, 0.6, 1.2},
			},
			wantAction: logic.ActionMarket,
			wantSide:   trading.Buy,
		},
		{
			name: "entry_manifold_herd",
			held: false,
			timeline: []measSpec{
				{logic.SourceManifold, logic.CategorySystemicHerd, 0.6, 1.2},
				{logic.SourceCorrelation, logic.CategorySystemicHerd, 0.6, 1.2},
				{logic.SourceSentiment, logic.CategoryRiskOnSurge, 0.6, 1.2},
			},
			wantAction: logic.ActionMarket,
			wantSide:   trading.Buy,
		},
		{
			name: "entry_scarcity",
			held: false,
			timeline: []measSpec{
				{logic.SourceLiquidity, logic.CategoryExtremeScarcity, 0.6, 1.2},
				{logic.SourcePumpDump, logic.CategoryOrganicTrend, 0.6, 1.2},
				{logic.SourcePumpDump, logic.CategoryCoiledCompression, 0.6, 1.2},
				{logic.SourcePumpDump, logic.CategoryVerticalIgnition, 0.6, 1.2},
				{logic.SourceHawkes, logic.CategoryFrenzy, 0.6, 1.2},
				{logic.SourceCausal, logic.CategoryEndogenousAlpha, 0.6, 1.2},
			},
			wantAction: logic.ActionMarket,
			wantSide:   trading.Buy,
		},
	}
}

func evaluateScenario(
	tree *logic.Tree, scenario treeScenario,
) (*logic.Evaluation, error) {
	holdings := logic.NewHoldings()

	if scenario.held {
		holdings.SetPosition(testSymbol, 1, 0)
	}

	base := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	measurements, measurementsErr := buildScenarioMeasurements(scenario, base)

	if measurementsErr != nil {
		return nil, measurementsErr
	}

	evaluation, err := tree.Evaluate(measurements, holdings)

	return evaluation, err
}

func buildScenarioMeasurements(
	scenario treeScenario,
	base time.Time,
) ([]logic.Measurement, error) {
	measurements := make([]logic.Measurement, 0, len(scenario.timeline))

	for index, spec := range scenario.timeline {
		measurement, measurementErr := synthMeasurement(
			spec.source,
			spec.category,
			spec.confidence,
			spec.surprise,
			base.Add(time.Duration(index)*time.Second),
		)

		if measurementErr != nil {
			return nil, measurementErr
		}

		measurements = append(measurements, measurement)
	}

	return measurements, nil
}

func synthMeasurement(
	source logic.SourceType,
	category logic.CategoryType,
	confidence float64,
	surprise float64,
	at time.Time,
) (logic.Measurement, error) {
	row, rowErr := market.NewSymbolRow(testSymbol, 50000, 0.01, 50000, 1, at)

	if rowErr != nil {
		return logic.Measurement{}, rowErr
	}

	measurement := logic.NewMeasurement(
		source,
		testSymbol,
		50000,
		0.8,
		100,
		1,
		1,
		category,
		logic.RegimeTypeNone,
		logic.PositionTypeNone,
		confidence,
		surprise,
	)
	measurement.ObservedAt = at
	measurement.Market = *row

	return measurement, nil
}
