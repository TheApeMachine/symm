package strategy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestPlannerPreDecision(t *testing.T) {
	Convey("Given an uncalibrated directional forecast", t, func() {
		planner := &Planner{minimumProbability: 0.7}
		forecast := &directionalForecast{
			symbol: "TEST/USD", at: time.Unix(1, 0),
			probabilityUp: 0.9, probabilityProfitable: 0.8,
			directionReady: false, profitabilityReady: false,
			directionSkillLowerBound: 0.1, profitSkillLowerBound: 0.2,
			directionCalibration: 0, profitCalibration: 0,
			directionFeatures: 0, profitFeatures: 0,
		}

		Convey("it still emits a well-formed hold round with a truthful status", func() {
			decision, round := planner.preDecision(forecast)
			So(decision, ShouldNotBeNil)
			So(round, ShouldNotBeNil)
			So(decision.Action, ShouldEqual, types.ActionNothing)
			So(decision.PredictiveReady, ShouldBeFalse)
			So(decision.PredictiveStatus, ShouldEqual, "uncalibrated-direction-and-profitability")
			So(round.Evaluated, ShouldBeTrue)
			So(round.Outcome, ShouldEqual, "admission")
			So(round.Decisions, ShouldHaveLength, 1)
		})
	})
}

func TestPlannerDecide(t *testing.T) {
	Convey("Given calibrated upward and profitability classifications", t, func() {
		planner := &Planner{minimumProbability: 0.7}
		forecast := &directionalForecast{
			symbol: "TEST/USD", at: time.Unix(1, 0),
			probabilityUp: 0.9, probabilityProfitable: 0.8,
			directionReady: true, profitabilityReady: true,
			directionSkillLowerBound: 0.1, profitSkillLowerBound: 0.2,
			directionCalibration: 100, profitCalibration: 90,
			directionFeatures: 320, profitFeatures: 320,
		}

		Convey("both probabilities must clear admission independently", func() {
			decision := planner.decide(forecast)
			So(decision.Action, ShouldEqual, types.ActionEnter)
			So(decision.Confidence, ShouldEqual, 0.8)
			_, predictsAmount := decision.Alternatives["expected_return"]
			So(predictsAmount, ShouldBeFalse)
		})

		Convey("a selected symbol below policy remains out", func() {
			forecast.probabilityProfitable = 0.69
			decision := planner.decide(forecast)
			So(decision.Action, ShouldEqual, types.ActionNothing)
		})
	})
}
