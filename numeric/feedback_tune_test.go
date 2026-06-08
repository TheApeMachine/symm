package numeric

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFeedbackTunerApply(t *testing.T) {
	Convey("Given matching feedback with rising samples", t, func() {
		tuner := NewFeedbackTuner()
		weights := DefaultClassifierWeights(2.0)
		baseline := weights.WIgnVol

		applied, err := tuner.Apply("ETH/EUR", "ETH/EUR", 4, 0.5, 1.0, 0.2, &weights)

		Convey("It should tune weights and threshold", func() {
			So(err, ShouldBeNil)
			So(applied, ShouldBeTrue)
			So(weights.Threshold, ShouldBeGreaterThan, 2.0)
			So(weights.WIgnVol, ShouldNotEqual, baseline)
		})
	})

	Convey("Given stale feedback samples", t, func() {
		tuner := NewFeedbackTuner()
		weights := DefaultClassifierWeights(2.0)

		_, applyErr := tuner.Apply("ETH/EUR", "ETH/EUR", 4, 0.5, 1.0, 0.2, &weights)
		So(applyErr, ShouldBeNil)

		applied, err := tuner.Apply("ETH/EUR", "ETH/EUR", 4, 0.5, 1.0, 0.2, &weights)

		Convey("It should skip without error", func() {
			So(err, ShouldBeNil)
			So(applied, ShouldBeFalse)
		})
	})

	Convey("Given nil weights", t, func() {
		tuner := NewFeedbackTuner()

		applied, err := tuner.Apply("ETH/EUR", "ETH/EUR", 4, 0.5, 1.0, 0.2, nil)

		Convey("It should return an error", func() {
			So(err, ShouldNotBeNil)
			So(applied, ShouldBeFalse)
		})
	})
}

func BenchmarkFeedbackTunerApply(b *testing.B) {
	tuner := NewFeedbackTuner()
	weights := DefaultClassifierWeights(2.0)

	b.ReportAllocs()

	samples := 0

	for b.Loop() {
		weights = DefaultClassifierWeights(2.0)
		samples++
		_, err := tuner.Apply("ETH/EUR", "ETH/EUR", samples, 0.5, 1.0, 0.2, &weights)

		if err != nil {
			b.Fatal(err)
		}
	}
}
