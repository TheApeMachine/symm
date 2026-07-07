package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDecisionGateForecast(testingTB *testing.T) {
	Convey("Given the decision gate and otherwise-allowing evidence", testingTB, func() {
		gate := newDecisionGate()

		Convey("When the supervised forecast is positive", func() {
			evidence := actionEvidence(1)
			evidence.predictive.forecast = 0.4

			result := gate.Decide(evidence)

			Convey("Then the gate allows the entry", func() {
				So(result.verdict, ShouldEqual, "allow")
			})
		})

		Convey("When the supervised forecast is negative", func() {
			evidence := actionEvidence(1)
			evidence.predictive.forecast = -0.2

			result := gate.Decide(evidence)

			Convey("Then the forecast rule blocks the entry", func() {
				So(result.verdict, ShouldEqual, "blocked")
				So(result.reason, ShouldEqual, "predictive_forecast_negative")
			})
		})

		Convey("When the forecast is zero (head warming up)", func() {
			evidence := actionEvidence(1)
			evidence.predictive.forecast = 0

			result := gate.Decide(evidence)

			Convey("Then the forecast rule does not block", func() {
				So(result.verdict, ShouldEqual, "allow")
			})
		})
	})
}
