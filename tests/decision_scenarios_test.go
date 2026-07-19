package tests_test

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
)

/*
TestDecisionScenarioRun runs enter → manage → exit proofs across named
market-simulation conditions and checks DecisionResults.Causes limbs.
*/
func TestDecisionScenarioRun(t *testing.T) {
	Convey("Given enter→exit DecisionScenario tapes", t, func() {
		symbol := conditions.Subject()
		scenarios := []tests.DecisionScenario{
			{
				Name:             "pump_take_profit",
				Market:           func() *tests.Market { return conditions.Pump(24, 12, 1.25, 8) },
				Symbol:           symbol,
				EnterExpected:    0.12,
				EnterUncertainty: 0.02,
				WantEnter:        true,
				Exit:             tests.ExitTakeProfit,
				PeakMul:          1.08,
				ExitExpected:     -0.02,
				WantCashLock:     true,
			},
			{
				Name:             "calm_take_profit",
				Market:           func() *tests.Market { return conditions.Calm(24) },
				Symbol:           symbol,
				EnterExpected:    0.10,
				EnterUncertainty: 0.02,
				WantEnter:        true,
				Exit:             tests.ExitTakeProfit,
				PeakMul:          1.06,
				ExitExpected:     -0.01,
				WantCashLock:     true,
			},
			{
				Name:             "aggression_take_profit",
				Market:           func() *tests.Market { return conditions.Aggression(24, 8, 4) },
				Symbol:           symbol,
				EnterExpected:    0.11,
				EnterUncertainty: 0.02,
				WantEnter:        true,
				Exit:             tests.ExitTakeProfit,
				PeakMul:          1.07,
				ExitExpected:     -0.02,
				WantCashLock:     true,
			},
			{
				Name:             "herd_take_profit",
				Market:           func() *tests.Market { return conditions.Herd(24) },
				Symbol:           symbol,
				EnterExpected:    0.10,
				EnterUncertainty: 0.02,
				WantEnter:        true,
				Exit:             tests.ExitTakeProfit,
				PeakMul:          1.06,
				ExitExpected:     -0.015,
				WantCashLock:     true,
			},
			{
				Name:             "imbalance_take_profit",
				Market:           func() *tests.Market { return conditions.Imbalance(24, 6, 3, 0.3) },
				Symbol:           symbol,
				EnterExpected:    0.10,
				EnterUncertainty: 0.025,
				WantEnter:        true,
				Exit:             tests.ExitTakeProfit,
				PeakMul:          1.05,
				ExitExpected:     -0.01,
				WantCashLock:     true,
			},
			{
				Name:             "drawdown_stop",
				Market:           func() *tests.Market { return conditions.Drawdown(24, 0.12, 10) },
				Symbol:           symbol,
				EnterExpected:    0.08,
				EnterUncertainty: 0.03,
				WantEnter:        true,
				Exit:             tests.ExitStop,
				PeakMul:          1.02,
				BreachMul:        0.97,
				ExitExpected:     0.01,
				WantCashLock:     false,
			},
			{
				Name:             "reversal_stop",
				Market:           func() *tests.Market { return conditions.Reversal(24, 10, 0.04) },
				Symbol:           symbol,
				EnterExpected:    0.09,
				EnterUncertainty: 0.025,
				WantEnter:        true,
				Exit:             tests.ExitStop,
				PeakMul:          1.04,
				BreachMul:        0.96,
				ExitExpected:     0.02,
				WantCashLock:     false,
			},
			{
				Name:             "pump_hold_while_forward_alive",
				Market:           func() *tests.Market { return conditions.Pump(24, 12, 1.25, 8) },
				Symbol:           symbol,
				EnterExpected:    0.12,
				EnterUncertainty: 0.02,
				WantEnter:        true,
				Exit:             tests.ExitNone,
				PeakMul:          1.08,
				ExitExpected:     0.05,
				ExitUncertainty:  0.01,
				WantCashLock:     false,
			},
			{
				Name:             "decay_hold_while_forward_alive",
				Market:           func() *tests.Market { return conditions.Decay(24, 8, 0.8) },
				Symbol:           symbol,
				EnterExpected:    0.09,
				EnterUncertainty: 0.03,
				WantEnter:        true,
				Exit:             tests.ExitNone,
				PeakMul:          1.04,
				ExitExpected:     0.04,
				WantCashLock:     false,
			},
			{
				Name:             "thin_herd_take_profit",
				Market:           func() *tests.Market { return conditions.ThinHerd(24, 0.25) },
				Symbol:           symbol,
				EnterExpected:    0.10,
				EnterUncertainty: 0.03,
				WantEnter:        true,
				Exit:             tests.ExitTakeProfit,
				PeakMul:          1.05,
				ExitExpected:     -0.02,
				WantCashLock:     true,
			},
			{
				Name:             "lag_take_profit",
				Market:           func() *tests.Market { return conditions.Lag(24, 3) },
				Symbol:           symbol,
				EnterExpected:    0.10,
				EnterUncertainty: 0.02,
				WantEnter:        true,
				Exit:             tests.ExitTakeProfit,
				PeakMul:          1.06,
				ExitExpected:     -0.015,
				WantCashLock:     true,
			},
			{
				Name:             "noise_take_profit",
				Market:           func() *tests.Market { return conditions.Noise(24) },
				Symbol:           symbol,
				EnterExpected:    0.10,
				EnterUncertainty: 0.03,
				WantEnter:        true,
				Exit:             tests.ExitTakeProfit,
				PeakMul:          1.05,
				ExitExpected:     -0.02,
				WantCashLock:     true,
			},
		}

		Convey("When each DecisionScenario.Run completes", func() {
			results := make(tests.DecisionResults, 0, len(scenarios))

			for _, scenario := range scenarios {
				result, err := scenario.Run(t, pumpdumpSignals)
				So(err, ShouldBeNil)
				results = append(results, result)
			}

			Convey("Then stop and take_profit limbs both fire across the suite", func() {
				counts := results.Causes()
				So(counts[string(tests.ExitStop)], ShouldBeGreaterThan, 0)
				So(counts[string(tests.ExitTakeProfit)], ShouldBeGreaterThan, 0)
				So(counts["none"], ShouldBeGreaterThan, 0)

				var summary strings.Builder
				summary.WriteString("\ndecision scenario summary\n")

				for _, result := range results {
					summary.WriteString(fmt.Sprintf(
						"%s enter=%t exit=%s open=%d\n",
						result.Name, result.Entered, result.ExitCause, result.OpenAfter,
					))
				}

				Println(summary.String())
			})
		})
	})
}

/*
BenchmarkSessionDecisionScenarios measures Play+Decide+exit across the core
tapes used for decision optimization.
*/
func BenchmarkSessionDecisionScenarios(b *testing.B) {
	symbol := conditions.Subject()
	core := []tests.DecisionScenario{
		{
			Name: "pump_take_profit",
			Market: func() *tests.Market {
				return conditions.Pump(16, 8, 1.25, 6)
			},
			Symbol: symbol, EnterExpected: 0.12, EnterUncertainty: 0.02,
			WantEnter: true, Exit: tests.ExitTakeProfit, PeakMul: 1.08,
			ExitExpected: -0.02, WantCashLock: true,
		},
		{
			Name: "drawdown_stop",
			Market: func() *tests.Market {
				return conditions.Drawdown(16, 0.12, 8)
			},
			Symbol: symbol, EnterExpected: 0.08, EnterUncertainty: 0.03,
			WantEnter: true, Exit: tests.ExitStop, PeakMul: 1.02, BreachMul: 0.97,
			ExitExpected: 0.01,
		},
		{
			Name: "pump_hold_while_forward_alive",
			Market: func() *tests.Market {
				return conditions.Pump(16, 8, 1.25, 6)
			},
			Symbol: symbol, EnterExpected: 0.12, EnterUncertainty: 0.02,
			WantEnter: true, Exit: tests.ExitNone, PeakMul: 1.08,
			ExitExpected: 0.05,
		},
	}

	b.ReportAllocs()

	for b.Loop() {
		for _, scenario := range core {
			if _, err := scenario.Run(b, pumpdumpSignals); err != nil {
				b.Fatal(err)
			}
		}
	}
}
