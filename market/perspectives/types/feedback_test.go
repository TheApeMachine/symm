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
			_, err := UpdateSourceFeedback(SourceDepthFlow, 0.01, 2, 4)
			So(err, ShouldBeNil)

			So(AdjustSourceValue(SourceDepthFlow, 0.4), ShouldAlmostEqual, 0.8, 1e-9)
		})

		Convey("It should soften an upstream source feature when settled truth was overcalled", func() {
			_, err := UpdateSourceFeedback(SourceDepthFlow, 0.01, 0.5, 4)
			So(err, ShouldBeNil)

			So(AdjustSourceValue(SourceDepthFlow, 0.4), ShouldAlmostEqual, 0.2, 1e-9)
		})
	})
}

func BenchmarkAdjustSourceValue(b *testing.B) {
	ResetSourceFeedback()
	defer ResetSourceFeedback()

	_, _ = UpdateSourceFeedback(SourceDepthFlow, 0.01, 1.25, 64)

	for b.Loop() {
		_ = AdjustSourceValue(SourceDepthFlow, 0.5)
	}
}
