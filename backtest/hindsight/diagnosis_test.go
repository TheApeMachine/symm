package hindsight

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDiagnoseOpportunity(t *testing.T) {
	leg := Leg{
		Symbol:    "TEST/USD",
		BuyAt:     time.Unix(1, 0).UTC(),
		SellAt:    time.Unix(2, 0).UTC(),
		BuyPrice:  100,
		SellPrice: 120,
		ProfitPct: 0.2,
	}

	Convey("Given a real current-architecture decision fixture", t, func() {
		context := SignalContext{
			Action:                   "nothing",
			Reason:                   "planner: valuation unavailable",
			Opportunity:              true,
			OpportunityType:          "pump",
			OpportunityPhase:         "detected",
			ValuationAttempted:       true,
			ValuationAvailable:       false,
			ValuationStatus:          "incomplete",
			CausalIdentification:     "",
			CausalBlockingCoordinate: "",
			UtilityAvailable:         true,
			Utility:                  0.1,
			MCTS: DecisionTrace{
				RecommendedAction: "nothing",
				Iterations:        40,
				Branches: []DecisionMCTSBranch{
					{Action: "enter", Visits: 10, MeanReward: 0.2},
					{Action: "cash", Visits: 30, MeanReward: 0.4},
				},
			},
			ProposedQuantity: Number(2),
			ProposedNotional: Number(200),
			AvailableCapital: Number(1000),
			EntryCost:        EntryCost{EntryPrice: Number(100), BestAsk: Number(100), BestBid: Number(99), EntryFee: Number(0.5)},
			Risk:             RiskPlan{Present: true, RiskDistance: Number(2), MaxLoss: Number(10), EntryFeeRate: Number(0.002), ExitFeeRate: Number(0.002)},
			ExpectedReturn:   Number(0.05),
			ExpectedFees:     Number(0.1),
			ExpectedSpread:   Number(0.01),
			ExpectedImpact:   Number(0.005),
			AdverseSelection: Number(0.002),
			Uncertainty:      0.3,
			Alternatives: map[string]float64{
				"meas:TEST/USD:depthflow:score": 0.4,
			},
		}

		diagnosis := diagnoseOpportunity(context, leg, true, true)

		Convey("It should report the valuation boundary, not a thesis score", func() {
			So(diagnosis.Category, ShouldEqual, DiagnosisValuation)
			So(diagnosis.Blockers[0].Key, ShouldEqual, "valuation:not_available")
			So(diagnosis.Recommendation.Kind, ShouldEqual, RecommendationCollectOutcomes)
		})
	})

	Convey("Given a missed leg without a retained same-symbol decision", t, func() {
		diagnosis := diagnoseOpportunity(SignalContext{}, leg, false, true)

		Convey("It should refuse to invent tuning advice", func() {
			So(diagnosis.Category, ShouldEqual, DiagnosisObservability)
			So(diagnosis.EvidenceStatus, ShouldEqual, "missing")
			So(diagnosis.Recommendation.Kind, ShouldEqual, RecommendationImproveAudit)
			So(diagnosis.Recommendation.HasSuggested, ShouldBeFalse)
			So(diagnosis.Summary, ShouldContainSubstring, "would be invented")
		})
	})

	Convey("Given an opposing measurement and no opportunity classification", t, func() {
		context := SignalContext{
			Action: "nothing",
			Alternatives: map[string]float64{
				"meas:TEST/USD:exhaustion:score": -0.8,
			},
		}

		diagnosis := diagnoseOpportunity(context, leg, true, true)

		Convey("It should prioritize detection over the generic classifier gap", func() {
			So(diagnosis.Blockers[0].Key, ShouldEqual, "detection:not_classified")
		})
	})

	Convey("Given a flagged opportunity while MCTS preferred CASH", t, func() {
		context := SignalContext{
			Action:               "nothing",
			Opportunity:          true,
			OpportunityType:      "pump",
			Reason:               "planner declined entry",
			ValuationAttempted:   true,
			ValuationAvailable:   true,
			ValuationStatus:      "complete",
			CausalIdentification: "identified",
			UtilityAvailable:     true,
			MCTS: DecisionTrace{
				RecommendedAction: "cash",
			},
			Alternatives: map[string]float64{},
		}

		diagnosis := diagnoseOpportunity(context, leg, true, true)

		Convey("It should show the selection boundary as the CASH preference", func() {
			So(diagnosis.Category, ShouldEqual, DiagnosisSelection)
			So(diagnosis.Blockers[0].Key, ShouldEqual, "selection:preferred_cash")
		})
	})

	Convey("Given a qualified opportunity delayed by the global regulator", t, func() {
		context := SignalContext{
			Action:               "nothing",
			Opportunity:          true,
			OpportunityType:      "pump",
			Reason:               "planner: entry delayed while global regulator is observing or adapting",
			ValuationAttempted:   true,
			ValuationAvailable:   true,
			ValuationStatus:      "complete",
			CausalIdentification: "identified",
			UtilityAvailable:     true,
			Alternatives: map[string]float64{
				admissionAcceptedKey: 1,
			},
		}

		diagnosis := diagnoseOpportunity(context, leg, true, true)

		Convey("It should name the regulator and propose a release-boundary replay", func() {
			So(diagnosis.Category, ShouldEqual, DiagnosisRegulator)
			So(diagnosis.Blockers[0].Key, ShouldEqual, "regulator:entry_delayed")
			So(diagnosis.Recommendation.Kind, ShouldEqual, RecommendationValidateRegulator)
		})
	})

	Convey("Given a slot rejection with retained allocation state", t, func() {
		context := SignalContext{
			Action:               "nothing",
			Opportunity:          true,
			Reason:               "planner: no position slot available for allocation",
			ValuationAttempted:   true,
			ValuationAvailable:   true,
			ValuationStatus:      "complete",
			CausalIdentification: "identified",
			UtilityAvailable:     true,
			OpenPositions:        4,
			SlotCapacity:         4,
			AllocationClass:      "none",
			Alternatives: map[string]float64{
				admissionAcceptedKey: 1,
			},
		}

		diagnosis := diagnoseOpportunity(context, leg, true, true)

		Convey("It should expose the exact capacity deficit", func() {
			So(diagnosis.Category, ShouldEqual, DiagnosisAllocation)
			So(diagnosis.Blockers[0].Key, ShouldEqual, "allocation:slot_capacity")
			So(diagnosis.Blockers[0].Observed, ShouldEqual, 4.0)
			So(diagnosis.Blockers[0].Target, ShouldEqual, 5.0)
		})
	})

	Convey("Given a qualified opportunity whose requested order exceeded visible depth", t, func() {
		context := SignalContext{
			Action:               "nothing",
			Opportunity:          true,
			Reason:               "desk: visible ask depth cannot execute complete quantity",
			ValuationAttempted:   true,
			ValuationAvailable:   true,
			ValuationStatus:      "complete",
			CausalIdentification: "identified",
			UtilityAvailable:     true,
			ProposedQuantity:     Number(100),
			Alternatives: map[string]float64{
				admissionAcceptedKey: 1,
				executionCoverageKey: 0.6,
			},
		}

		diagnosis := diagnoseOpportunity(context, leg, true, true)

		Convey("It should quantify the execution shortfall", func() {
			So(diagnosis.Category, ShouldEqual, DiagnosisExecution)
			So(diagnosis.Blockers[0].Key, ShouldEqual, executionCoverageKey)
			So(diagnosis.Blockers[0].Target, ShouldEqual, completeExecutionCoverage)
			So(diagnosis.Blockers[0].Gap, ShouldAlmostEqual, 0.4, 1e-12)
		})
	})
}
