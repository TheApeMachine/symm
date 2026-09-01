package strategy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

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
			So(decision.ExpectedReturn, ShouldBeNil)
			So(decision.Confidence, ShouldEqual, 0.8)
		})

		Convey("a selected symbol below policy remains out", func() {
			forecast.probabilityProfitable = 0.69
			decision := planner.decide(forecast)
			So(decision.Action, ShouldEqual, types.ActionNothing)
		})
	})
}
