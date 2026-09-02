package strategy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestPlannerStep(t *testing.T) {
	Convey("Given a malformed ticker carrying otherwise valid observations", t, func() {
		predictor, err := newDirectionalPredictor(directionalConfig{
			initialVariance: 1, forgettingFactor: 1,
		})
		So(err, ShouldBeNil)
		planner := &Planner{predictor: predictor}
		envelope := fullObservationEnvelope()

		out := planner.Step(envelope)

		Convey("ticker integrity is established before predictor state mutates", func() {
			So(out, ShouldBeNil)
			So(planner.Error(), ShouldNotBeNil)
			So(predictor.states, ShouldBeEmpty)
			So(planner.Step(fullObservationEnvelope()), ShouldBeNil)
		})
	})
}

func TestPlannerPreDecision(t *testing.T) {
	Convey("Given an unresolved adaptive return distribution", t, func() {
		planner := &Planner{}
		forecast := &directionalForecast{
			symbol: "TEST/USD", at: time.Unix(1, 0),
			status: "awaiting-return-distribution", horizonSteps: 3,
			opportunity: types.OpportunityCandidate{
				Archetype: types.ArchetypeVerticalIgnition,
				Phase:     types.PhaseArmed,
			},
		}

		Convey("it emits a truthful non-action decision with its opportunity context", func() {
			decision, round := planner.preDecision(forecast)
			So(decision, ShouldNotBeNil)
			So(round, ShouldNotBeNil)
			So(decision.Action, ShouldEqual, types.ActionNothing)
			So(decision.PredictiveReady, ShouldBeFalse)
			So(decision.PredictiveStatus, ShouldEqual, "awaiting-return-distribution")
			So(decision.OpportunityType, ShouldEqual, string(types.ArchetypeVerticalIgnition))
			So(decision.OpportunityPhase, ShouldEqual, string(types.PhaseArmed))
			So(round.Evaluated, ShouldBeTrue)
			So(round.Decisions, ShouldHaveLength, 1)
		})
	})
}

func TestPlannerDecide(t *testing.T) {
	Convey("Given an opportunity-conditioned executable return distribution", t, func() {
		planner := &Planner{}
		forecast := &directionalForecast{
			symbol: "TEST/USD", at: time.Unix(1, 0), ready: true,
			status:        "adaptive-return-distribution-ready",
			probabilityUp: 0.8, probabilityProfitable: 0.7,
			expectedLogReturn: 0.02, breakEvenLogReturn: 0.01,
			opportunity: types.OpportunityCandidate{
				Archetype: types.ArchetypeVerticalIgnition,
				Phase:     types.PhaseArmed,
			},
		}

		Convey("entry wins only when its distribution center clears executable break-even", func() {
			decision := planner.decide(forecast)
			So(decision.Action, ShouldEqual, types.ActionEnter)
			So(decision.Confidence, ShouldEqual, 0.7)
			So(decision.Alternatives["return:expected_log"], ShouldEqual, 0.02)
			So(decision.Alternatives["return:break_even_log"], ShouldEqual, 0.01)
		})

		Convey("cash wins when the forecast does not cover current execution economics", func() {
			forecast.expectedLogReturn = forecast.breakEvenLogReturn
			decision := planner.decide(forecast)
			So(decision.Action, ShouldEqual, types.ActionNothing)
		})
	})
}
