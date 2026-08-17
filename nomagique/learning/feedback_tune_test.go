package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func testFeedbackWeights(testingTB *testing.T) ClassifierWeights {
	testingTB.Helper()

	weights, err := NewClassifierWeights(testClassifierConfig(), 2.0, testClassifierScales())

	if err != nil {
		testingTB.Fatal(err)
	}

	return weights
}

func TestFeedbackTunerApply(testingTB *testing.T) {
	Convey("Given matching feedback with rising samples", testingTB, func() {
		tuner := NewFeedbackTuner()
		weights := testFeedbackWeights(testingTB)
		baseline := weights.termWeights["ignition"]["rvol"]

		applied, err := tuner.Apply("member-a", "member-a", 4, 0.5, 1.0, 0.2, &weights)

		Convey("It should tune weights and threshold", func() {
			So(err, ShouldBeNil)
			So(applied, ShouldBeTrue)
			So(weights.Threshold, ShouldBeGreaterThan, 2.0)
			So(weights.termWeights["ignition"]["rvol"], ShouldNotEqual, baseline)
		})
	})

	Convey("Given stale feedback samples", testingTB, func() {
		tuner := NewFeedbackTuner()
		weights := testFeedbackWeights(testingTB)

		_, applyErr := tuner.Apply("member-a", "member-a", 4, 0.5, 1.0, 0.2, &weights)
		So(applyErr, ShouldBeNil)

		applied, err := tuner.Apply("member-a", "member-a", 4, 0.5, 1.0, 0.2, &weights)

		Convey("It should skip without error", func() {
			So(err, ShouldBeNil)
			So(applied, ShouldBeFalse)
		})
	})

	Convey("Given nil weights", testingTB, func() {
		tuner := NewFeedbackTuner()

		applied, err := tuner.Apply("member-a", "member-a", 4, 0.5, 1.0, 0.2, nil)

		Convey("It should return an error", func() {
			So(err, ShouldNotBeNil)
			So(applied, ShouldBeFalse)
		})
	})
}

func BenchmarkFeedbackTunerApply(b *testing.B) {
	tuner := NewFeedbackTuner()

	b.ReportAllocs()

	samples := 0

	for b.Loop() {
		weights, err := NewClassifierWeights(testClassifierConfig(), 2.0, testClassifierScales())

		if err != nil {
			b.Fatal(err)
		}

		samples++
		_, err = tuner.Apply("member-a", "member-a", samples, 0.5, 1.0, 0.2, &weights)

		if err != nil {
			b.Fatal(err)
		}
	}
}
