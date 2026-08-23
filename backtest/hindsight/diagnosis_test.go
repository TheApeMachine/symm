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

	Convey("Given a retained decision with one exact failed admission margin", t, func() {
		context := SignalContext{
			Action:             "nothing",
			Reason:             "planner: structural thesis below admission",
			GraphScore:         0.7,
			AdmissionThreshold: 0.5,
			ThesisScore:        0.48,
			Direction:          1,
			Opportunity:        true,
			PredictiveReady:    true,
			Alternatives: map[string]float64{
				admissionThesisMarginKey:        -0.02,
				admissionAcceptedKey:            0,
				"meas:TEST/USD:depthflow:score": 0.4,
			},
		}

		diagnosis := diagnoseOpportunity(context, leg, true)

		Convey("It should preserve the causal boundary and propose a counterfactual sweep", func() {
			So(diagnosis.Category, ShouldEqual, DiagnosisAdmission)
			So(diagnosis.Blockers, ShouldHaveLength, 2)
			So(diagnosis.Blockers[0].Key, ShouldEqual, "admission:thesis_score")
			So(diagnosis.Blockers[0].Observed, ShouldAlmostEqual, 0.48, 1e-12)
			So(diagnosis.Blockers[0].Target, ShouldAlmostEqual, 0.5, 1e-12)
			So(diagnosis.Recommendation.Kind, ShouldEqual, RecommendationTuneParameter)
			So(diagnosis.Recommendation.Current, ShouldAlmostEqual, 0.5, 1e-12)
			So(diagnosis.Recommendation.Suggested, ShouldAlmostEqual, 0.48, 1e-12)
			So(diagnosis.Recommendation.Action, ShouldContainSubstring, "wallet loss")
		})
	})

	Convey("Given a missed leg without a retained same-symbol decision", t, func() {
		diagnosis := diagnoseOpportunity(SignalContext{}, leg, false)

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
			Action:          "nothing",
			PredictiveReady: true,
			Alternatives: map[string]float64{
				"meas:TEST/USD:exhaustion:score": -0.8,
			},
		}

		diagnosis := diagnoseOpportunity(context, leg, true)

		Convey("It should prioritize the recorded measurement over the generic classifier gap", func() {
			So(diagnosis.Category, ShouldEqual, DiagnosisMeasurement)
			So(diagnosis.Blockers[0].Source, ShouldEqual, "exhaustion")
			So(diagnosis.Recommendation.Kind, ShouldEqual, RecommendationImproveMeasurement)
			So(diagnosis.Recommendation.Action, ShouldContainSubstring, "held-out")
		})
	})

	Convey("Given a flagged opportunity while predictive coding is not ready", t, func() {
		context := SignalContext{
			Action:           "nothing",
			Opportunity:      true,
			Type:             "pump",
			Reason:           "planner declined entry",
			PredictiveReady:  false,
			PredictiveStatus: "still calibrating",
			Alternatives:     map[string]float64{},
		}

		diagnosis := diagnoseOpportunity(context, leg, true)

		Convey("It should show readiness as supporting evidence without blaming it as the veto", func() {
			So(diagnosis.Category, ShouldEqual, DiagnosisFollowThrough)
			So(diagnosis.Blockers, ShouldHaveLength, 2)
			So(diagnosis.Blockers[0].Key, ShouldEqual, "opportunity:flagged_without_entry")
			So(diagnosis.Blockers[1].Key, ShouldEqual, "predictive:not_ready")
			So(diagnosis.Recommendation.Kind, ShouldEqual, RecommendationInstrumentFunnel)
		})
	})

	Convey("Given a qualified opportunity delayed by the global regulator", t, func() {
		context := SignalContext{
			Action:          "nothing",
			Opportunity:     true,
			Type:            "pump",
			Reason:          "planner: entry delayed while global regulator is observing or adapting",
			PredictiveReady: true,
			Alternatives: map[string]float64{
				admissionAcceptedKey: 1,
			},
		}

		diagnosis := diagnoseOpportunity(context, leg, true)

		Convey("It should name the regulator and propose a release-boundary replay", func() {
			So(diagnosis.Category, ShouldEqual, DiagnosisRegulator)
			So(diagnosis.Blockers[0].Key, ShouldEqual, "regulator:entry_delayed")
			So(diagnosis.Recommendation.Kind, ShouldEqual, RecommendationValidateRegulator)
			So(diagnosis.Recommendation.Action, ShouldContainSubstring, "earlier release")
		})
	})

	Convey("Given a slot rejection with retained allocation state", t, func() {
		context := SignalContext{
			Action:          "nothing",
			Opportunity:     true,
			Reason:          "planner: no position slot available for allocation",
			PredictiveReady: true,
			OpenPositions:   4,
			SlotCapacity:    4,
			AllocationClass: "none",
			Alternatives: map[string]float64{
				admissionAcceptedKey: 1,
			},
		}

		diagnosis := diagnoseOpportunity(context, leg, true)

		Convey("It should expose the exact capacity deficit before generic allocation prose", func() {
			So(diagnosis.Category, ShouldEqual, DiagnosisAllocation)
			So(diagnosis.Blockers[0].Key, ShouldEqual, "allocation:slot_capacity")
			So(diagnosis.Blockers[0].Observed, ShouldEqual, 4.0)
			So(diagnosis.Blockers[0].Target, ShouldEqual, 5.0)
			So(diagnosis.Recommendation.Kind, ShouldEqual, RecommendationFixAllocation)
		})
	})

	Convey("Given an admission rejection retained only as prose", t, func() {
		context := SignalContext{
			Action:          "nothing",
			Reason:          "planner: entry no longer satisfies admission",
			PredictiveReady: true,
		}

		diagnosis := diagnoseOpportunity(context, leg, true)

		Convey("It should request numerical gate evidence rather than invent a zero boundary", func() {
			So(diagnosis.Category, ShouldEqual, DiagnosisAdmission)
			So(diagnosis.Recommendation.Kind, ShouldEqual, RecommendationImproveAudit)
			So(diagnosis.Recommendation.HasCurrent, ShouldBeFalse)
			So(diagnosis.Recommendation.HasSuggested, ShouldBeFalse)
			So(diagnosis.Recommendation.Action, ShouldContainSubstring, "signed margin")
		})
	})

	Convey("Given a qualified opportunity whose requested order exceeded visible depth", t, func() {
		context := SignalContext{
			Action:          "nothing",
			Opportunity:     true,
			Reason:          "desk: visible ask depth cannot execute complete quantity",
			PredictiveReady: true,
			Alternatives: map[string]float64{
				admissionAcceptedKey: 1,
				executionCoverageKey: 0.6,
			},
		}

		diagnosis := diagnoseOpportunity(context, leg, true)

		Convey("It should quantify the execution shortfall and recommend a smaller-order replay", func() {
			So(diagnosis.Category, ShouldEqual, DiagnosisExecution)
			So(diagnosis.Blockers[0].Key, ShouldEqual, executionCoverageKey)
			So(diagnosis.Blockers[0].Target, ShouldEqual, completeExecutionCoverage)
			So(diagnosis.Blockers[0].Gap, ShouldAlmostEqual, 0.4, 1e-12)
			So(diagnosis.Recommendation.Kind, ShouldEqual, RecommendationFixExecution)
			So(diagnosis.Recommendation.Action, ShouldContainSubstring, "smaller executable order")
		})
	})

	Convey("Given a failed direction gate with its configured target retained", t, func() {
		context := SignalContext{
			Action:          "nothing",
			Direction:       0.5,
			Opportunity:     true,
			PredictiveReady: true,
			Alternatives: map[string]float64{
				admissionDirectionMarginKey: -0.25,
				admissionDirectionTargetKey: 0.75,
			},
		}

		diagnosis := diagnoseOpportunity(context, leg, true)

		Convey("It should report the recorded policy target without assuming a fixed long direction", func() {
			So(diagnosis.Category, ShouldEqual, DiagnosisDirection)
			So(diagnosis.Blockers[0].Observed, ShouldAlmostEqual, 0.5, 1e-12)
			So(diagnosis.Blockers[0].Target, ShouldAlmostEqual, 0.75, 1e-12)
			So(diagnosis.Blockers[0].Gap, ShouldAlmostEqual, 0.25, 1e-12)
			So(diagnosis.Recommendation.Kind, ShouldEqual, RecommendationImproveMeasurement)
		})
	})

}
