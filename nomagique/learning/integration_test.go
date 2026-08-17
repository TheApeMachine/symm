package learning_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/learning"
)

func TestLearningTypedIntegration(testingTB *testing.T) {
	Convey("Given typed learning stages", testingTB, func() {
		trust := learning.Weight()
		calibrator := learning.SampleRatio()
		forecaster := learning.Forecast()

		pair := learning.LearningPair{Predicted: 10, Actual: 10}
		trustOutput, trustErr := trust.Measure(pair)
		ratioOutput, ratioErr := calibrator.Measure(pair)
		forecastOutput, forecastErr := forecaster.Measure(pair)

		Convey("It should compose prediction outcomes without wire transport", func() {
			So(trustErr, ShouldBeNil)
			So(ratioErr, ShouldBeNil)
			So(forecastErr, ShouldBeNil)
			So(trustOutput.Value, ShouldEqual, 1)
			So(ratioOutput.Value, ShouldEqual, 1)
			So(forecastOutput.Value, ShouldEqual, 1)
		})
	})
}
