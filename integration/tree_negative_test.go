package integration

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/logic"
)

func TestTreeNegativeScenarios(test *testing.T) {
	tree, treeErr := logic.NewTree(nil)

	if treeErr != nil {
		test.Fatal(treeErr)
	}

	scenarios := map[string]treeScenario{}

	for _, scenario := range allTreeScenarios() {
		scenarios[scenario.name] = scenario
	}

	testCases := []treeScenario{
		withHeld(scenarios["exit_mechanical_collapse"], false),
		withHeld(scenarios["entry_ignition"], true),
		withWeakEvidence(scenarios["entry_ignition"]),
	}

	for _, testCase := range testCases {
		test.Run(testCase.name, func(test *testing.T) {
			evaluation, evalErr := evaluateScenario(tree, testCase)

			if evalErr != nil {
				test.Fatalf("evaluate scenario: %v", evalErr)
			}

			if evaluation != nil {
				test.Fatalf("expected no action, got %#v", evaluation.Action)
			}
		})
	}
}

func TestTreeMeasurementEligibilityNegativeInputs(test *testing.T) {
	referenceAt := time.Date(2026, 6, 11, 12, 0, 2, 0, time.UTC)
	maxAge := time.Second
	fresh := synthMeasurement(
		logic.SourcePumpDump,
		logic.CategoryOrganicTrend,
		0.6,
		1.2,
		referenceAt,
	)

	testCases := []struct {
		name        string
		measurement logic.Measurement
	}{
		{
			name:        "stale_measurement",
			measurement: withObservedAt(fresh, referenceAt.Add(-2*time.Second)),
		},
		{
			name:        "missing_price",
			measurement: withPrice(fresh, 0),
		},
		{
			name:        "missing_symbol",
			measurement: withSymbol(fresh, ""),
		},
	}

	for _, testCase := range testCases {
		test.Run(testCase.name, func(test *testing.T) {
			if testCase.measurement.DecisionEligible(referenceAt, maxAge) {
				test.Fatalf("expected %s to be ineligible", testCase.name)
			}
		})
	}
}

func withHeld(scenario treeScenario, held bool) treeScenario {
	scenario.held = held
	scenario.name += "_wrong_holding_state"

	return scenario
}

func withWeakEvidence(scenario treeScenario) treeScenario {
	scenario.name += "_weak_evidence"

	for index := range scenario.timeline {
		scenario.timeline[index].confidence = 0.01
		scenario.timeline[index].surprise = 0.01
	}

	return scenario
}

func withObservedAt(
	measurement logic.Measurement,
	observedAt time.Time,
) logic.Measurement {
	measurement.ObservedAt = observedAt

	return measurement
}

func withPrice(
	measurement logic.Measurement,
	price float64,
) logic.Measurement {
	measurement.Price = price

	return measurement
}

func withSymbol(
	measurement logic.Measurement,
	symbol string,
) logic.Measurement {
	measurement.Symbol = symbol

	return measurement
}
