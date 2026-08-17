package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWeight(testingTB *testing.T) {
	Convey("Given Weight constructor", testingTB, func() {
		trustWeight := Weight()

		Convey("It should return a usable learner", func() {
			So(trustWeight, ShouldNotBeNil)
		})
	})
}

func TestTrustWeightMeasure(testingTB *testing.T) {
	Convey("Given a fresh trust weight", testingTB, func() {
		trustWeight := Weight()
		output, err := trustWeight.Measure(LearningPair{Predicted: 10, Actual: 10})

		Convey("It should return full trust", func() {
			So(err, ShouldBeNil)
			So(output.Value, ShouldEqual, 1)
			So(output.Count, ShouldEqual, 1)
		})
	})

	Convey("Given diverging outcomes", testingTB, func() {
		trustWeight := Weight()
		_, err := trustWeight.Measure(LearningPair{Predicted: 10, Actual: 10})
		So(err, ShouldBeNil)

		output, err := trustWeight.Measure(LearningPair{Predicted: 20, Actual: 30})

		Convey("It should reduce trust", func() {
			So(err, ShouldBeNil)
			So(output.Value, ShouldBeLessThan, 1)
		})
	})

	Convey("Given zero predicted", testingTB, func() {
		trustWeight := Weight()
		_, err := trustWeight.Measure(LearningPair{Predicted: 0, Actual: 10})

		Convey("It should return a parse error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestTrustWeightReset(testingTB *testing.T) {
	Convey("Given trust weight with state", testingTB, func() {
		trustWeight := Weight()
		_, err := trustWeight.Measure(LearningPair{Predicted: 10, Actual: 10})
		So(err, ShouldBeNil)

		trustWeight.Reset()
		output, err := trustWeight.Measure(LearningPair{Predicted: 10, Actual: 10})

		Convey("It should observe again after reset", func() {
			So(err, ShouldBeNil)
			So(output.Count, ShouldEqual, 1)
			So(output.Value, ShouldEqual, 1)
		})
	})
}

func BenchmarkTrustWeightMeasure(testingTB *testing.B) {
	trustWeight := Weight()
	_, _ = trustWeight.Measure(LearningPair{Predicted: 10, Actual: 10})

	testingTB.ReportAllocs()

	for testingTB.Loop() {
		_, _ = trustWeight.Measure(LearningPair{Predicted: 10, Actual: 11})
	}
}
