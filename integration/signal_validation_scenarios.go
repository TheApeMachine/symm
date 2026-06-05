package integration

import (
	"fmt"
	"time"

	"github.com/theapemachine/symm/market/perspectives"
)

/*
signalValidationScenarios emits one replay scenario per signal category probe.
*/
func signalValidationScenarios() []Scenario {
	probes := signalCategoryProbes()
	scenarios := make([]Scenario, 0, len(probes))

	for _, probe := range probes {
		if probe.Scenario != nil {
			scenarios = append(scenarios, probe.Scenario(probe))

			continue
		}

		scenarios = append(scenarios, defaultSignalValidationScenario(probe))
	}

	return scenarios
}

func defaultSignalValidationScenario(probe SignalCategoryProbe) Scenario {
	return Scenario{
		ID:   signalValidationScenarioID(probe),
		Name: signalValidationScenarioName(probe),
		BuildCapture: func(builder *CaptureBuilder) {
			builder.ApplySignalFixture(probe.Fixture)
		},
		SettleDelay: signalValidationSettleDelay(probe.Source),
		Checks: []ScenarioCheck{
			checkSignalCategoryFixture(
				"signal.category",
				fmt.Sprintf(
					"%s latest reading on %s must be %s",
					probe.Source,
					probe.Symbol,
					probe.Category,
				),
				probe,
			),
		},
	}
}

func signalValidationScenarioID(probe SignalCategoryProbe) string {
	return fmt.Sprintf(
		"signal.validate.%s.%s",
		probe.Source,
		probe.Category,
	)
}

func signalValidationScenarioName(probe SignalCategoryProbe) string {
	return fmt.Sprintf(
		"%s → %s when %s",
		probe.Source,
		probe.Category,
		probe.Condition,
	)
}

func signalValidationSettleDelay(source perspectives.SourceType) time.Duration {
	switch source {
	case perspectives.SourceCausal, perspectives.SourceCorrelation, perspectives.SourceLeadLag:
		return 2 * time.Second
	case perspectives.SourceHawkes:
		return 900 * time.Millisecond
	default:
		return 700 * time.Millisecond
	}
}

func causalNoiseScenario(probe SignalCategoryProbe) Scenario {
	scenario := defaultSignalValidationScenario(probe)
	scenario.PostReplayTickers = causalNoisePostReplayTickers()
	scenario.PostReplayTrades = causalNoisePostReplayTrades()
	scenario.PostReplayPace = 150 * time.Millisecond
	scenario.RunTimeout = 8 * time.Second
	scenario.SettleDelay = 2 * time.Second

	return scenario
}

func leadlagAnchorStallScenario(probe SignalCategoryProbe) Scenario {
	scenario := defaultSignalValidationScenario(probe)
	scenario.PostReplayTickers = leadLagPostReplayTickers()
	scenario.PostReplayPace = 220 * time.Millisecond
	scenario.RunTimeout = 12 * time.Second
	scenario.SettleDelay = 1500 * time.Millisecond

	return scenario
}

func leadlagInefficientLagScenario(probe SignalCategoryProbe) Scenario {
	scenario := defaultSignalValidationScenario(probe)
	scenario.PostReplayTickers = leadLagInefficientLagTickers()
	scenario.PostReplayPace = 220 * time.Millisecond
	scenario.RunTimeout = 12 * time.Second
	scenario.SettleDelay = 1500 * time.Millisecond

	return scenario
}

func leadlagSynchronizedDriftScenario(probe SignalCategoryProbe) Scenario {
	scenario := defaultSignalValidationScenario(probe)
	scenario.PostReplayTickers = leadLagSynchronizedDriftTickers()
	scenario.PostReplayPace = 220 * time.Millisecond
	scenario.RunTimeout = 12 * time.Second
	scenario.SettleDelay = 1500 * time.Millisecond

	return scenario
}

func leadlagDecoupledMoveScenario(probe SignalCategoryProbe) Scenario {
	scenario := defaultSignalValidationScenario(probe)
	scenario.PostReplayTickers = leadLagDecoupledMoveTickers()
	scenario.PostReplayPace = 220 * time.Millisecond
	scenario.RunTimeout = 12 * time.Second
	scenario.SettleDelay = 1500 * time.Millisecond

	return scenario
}

func correlationHerdScenario(probe SignalCategoryProbe) Scenario {
	scenario := defaultSignalValidationScenario(probe)
	scenario.PostReplayTradeBatches = correlationPostReplayTradeBatches()
	scenario.PostReplayPace = 250 * time.Millisecond
	scenario.RunTimeout = 15 * time.Second
	scenario.SettleDelay = 2 * time.Second

	return scenario
}

func correlationDecoupledScenario(probe SignalCategoryProbe) Scenario {
	scenario := defaultSignalValidationScenario(probe)
	scenario.BuildCapture = func(builder *CaptureBuilder) {
		builder.ApplySignalFixture(probe.Fixture)
	}
	scenario.PostReplayTradeBatches = correlationDecoupledTradeBatches()
	scenario.PostReplayPace = 250 * time.Millisecond
	scenario.RunTimeout = 15 * time.Second
	scenario.SettleDelay = 2 * time.Second

	return scenario
}

func correlationNoiseScenario(probe SignalCategoryProbe) Scenario {
	scenario := defaultSignalValidationScenario(probe)
	scenario.PostReplayTradeBatches = correlationNoiseTradeBatches()
	scenario.PostReplayPace = 250 * time.Millisecond
	scenario.RunTimeout = 15 * time.Second
	scenario.SettleDelay = 2 * time.Second

	return scenario
}

func correlationDivergentStressScenario(probe SignalCategoryProbe) Scenario {
	scenario := defaultSignalValidationScenario(probe)
	scenario.PostReplayTradeBatches = correlationDivergentStressTradeBatches()
	scenario.PostReplayPace = 250 * time.Millisecond
	scenario.RunTimeout = 15 * time.Second
	scenario.SettleDelay = 2 * time.Second

	return scenario
}
