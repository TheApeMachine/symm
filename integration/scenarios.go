package integration

import (
	"time"

	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
ScenarioCheck evaluates one assertion against collected telemetry.
*/
type ScenarioCheck struct {
	ID       string
	Name     string
	Evaluate func(snapshot TapeSnapshot, engineErr error) (bool, string, map[string]any)
}

/*
Scenario is one deterministic replay-backed integration path.
*/
type Scenario struct {
	ID                     string
	Name                   string
	BuildCapture           func(*CaptureBuilder)
	DirectMeasurements     []types.Measurement
	HoldingSymbols         []string
	PostReplayTrades       []market.TradeUpdate
	PostReplayTradeBatches [][]market.TradeUpdate
	PostReplayTickers      []market.TickerUpdate
	PreDirectTickers       []market.TickerUpdate
	PreDirectBooks         []market.Book
	PostOrderTickers       []market.TickerUpdate
	PostOrderBooks         []market.Book
	PostOrderDelay         time.Duration
	PostReplayDelay        time.Duration
	PostReplayPace         time.Duration
	SettleDelay            time.Duration
	RunTimeout             time.Duration
	Checks                 []ScenarioCheck
}

func allScenarios() []Scenario {
	scenarios := make([]Scenario, 0, 64)
	scenarios = append(scenarios, infraScenarios()...)
	scenarios = append(scenarios, signalScenarios()...)
	scenarios = append(scenarios, playbookScenarios()...)
	scenarios = append(scenarios, executionScenarios()...)
	scenarios = append(scenarios, stressScenarios()...)

	return scenarios
}

func buildCapture(scenario Scenario) *CaptureBuilder {
	builder := NewCaptureBuilder(time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC))

	if scenario.BuildCapture != nil {
		scenario.BuildCapture(builder)
	}

	return builder
}

func allSignalSources() []types.SourceType {
	return []types.SourceType{
		types.SourceCVD,
		types.SourceFluid,
		types.SourceHawkes,
		types.SourceDepthFlow,
		types.SourceSentiment,
		types.SourceCorrelation,
		types.SourceCausal,
		types.SourceLeadLag,
		types.SourceLiquidity,
		types.SourceExhaustion,
		types.SourcePumpDump,
		types.SourceToxicity,
	}
}
