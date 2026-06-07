package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAdjustSourceValue(t *testing.T) {
	Convey("Given learned bidirectional source feedback", t, func() {
		ResetSourceFeedback()
		defer ResetSourceFeedback()

		Convey("It should sharpen an upstream source feature when settled truth was undercalled", func() {
			_, err := UpdateSourceFeedback(SourceDepthFlow, 0.01, 2, 0, 4)
			So(err, ShouldBeNil)

			So(AdjustSourceValue(SourceDepthFlow, 0.4), ShouldAlmostEqual, 0.8, 1e-9)
		})

		Convey("It should soften an upstream source feature when settled truth was overcalled", func() {
			_, err := UpdateSourceFeedback(SourceDepthFlow, 0.01, 0.5, 0, 4)
			So(err, ShouldBeNil)

			So(AdjustSourceValue(SourceDepthFlow, 0.4), ShouldAlmostEqual, 0.2, 1e-9)
		})

		Convey("It should clamp scale below 0.5 to the lower bound", func() {
			_, err := UpdateSourceFeedback(SourceDepthFlow, 0.01, 0.1, 0, 4)
			So(err, ShouldBeNil)

			So(AdjustSourceValue(SourceDepthFlow, 1.0), ShouldAlmostEqual, 0.5, 1e-9)
		})

		Convey("It should apply an in-range scale exactly", func() {
			_, err := UpdateSourceFeedback(SourceDepthFlow, 0.01, 1.25, 0, 4)
			So(err, ShouldBeNil)

			So(AdjustSourceValue(SourceDepthFlow, 0.8), ShouldAlmostEqual, 1.0, 1e-9)
		})

		Convey("It should clamp scale above 2 to the upper bound", func() {
			_, err := UpdateSourceFeedback(SourceDepthFlow, 0.01, 3.0, 0, 4)
			So(err, ShouldBeNil)

			So(AdjustSourceValue(SourceDepthFlow, 1.0), ShouldAlmostEqual, 2.0, 1e-9)
		})
	})
}

func BenchmarkAdjustSourceValue(b *testing.B) {
	ResetSourceFeedback()
	defer ResetSourceFeedback()

	_, _ = UpdateSourceFeedback(SourceDepthFlow, 0.01, 1.25, 0, 64)

	for b.Loop() {
		_ = AdjustSourceValue(SourceDepthFlow, 0.5)
	}
}
