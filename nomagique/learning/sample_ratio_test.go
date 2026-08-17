package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSampleRatio(testingTB *testing.T) {
	Convey("Given SampleRatio constructor", testingTB, func() {
		calibrator := SampleRatio()

		Convey("It should return a usable learner", func() {
			So(calibrator, ShouldNotBeNil)
		})
	})
}

func TestCalibratorMeasure(testingTB *testing.T) {
	Convey("Given equal predicted and actual values", testingTB, func() {
		calibrator := SampleRatio()
		output, err := calibrator.Measure(LearningPair{Predicted: 10, Actual: 10})

		Convey("It should return unit ratio", func() {
			So(err, ShouldBeNil)
			So(output.Value, ShouldEqual, 1)
		})
	})

	Convey("Given actual above predicted", testingTB, func() {
		calibrator := SampleRatio()
		_, _ = calibrator.Measure(LearningPair{Predicted: 10, Actual: 10})
		output, err := calibrator.Measure(LearningPair{Predicted: 10, Actual: 15})

		Convey("It should return a ratio above one", func() {
			So(err, ShouldBeNil)
			So(output.Value, ShouldBeGreaterThan, 1)
		})
	})

	Convey("Given zero predicted", testingTB, func() {
		calibrator := SampleRatio()
		_, err := calibrator.Measure(LearningPair{Predicted: 0, Actual: 10})

		Convey("It should return a validation error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkSampleRatioMeasure(testingTB *testing.B) {
	calibrator := SampleRatio()
	_, _ = calibrator.Measure(LearningPair{Predicted: 10, Actual: 10})

	testingTB.ReportAllocs()

	for testingTB.Loop() {
		_, _ = calibrator.Measure(LearningPair{Predicted: 10, Actual: 11})
	}
}
