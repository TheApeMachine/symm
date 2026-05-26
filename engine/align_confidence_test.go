package engine

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAlignConfidencePartialAlignment(t *testing.T) {
	Convey("Given two hallmark indicators firing weakly", t, func() {
		Convey("It should reflect partial measurement alignment below certainty", func() {
			confidence := AlignConfidence(0.5, 0.5)

			So(confidence, ShouldBeGreaterThan, 0)
			So(confidence, ShouldBeLessThan, 1)
		})
	})
}

func TestAlignConfidenceAllowsPartialAlignment(t *testing.T) {
	Convey("Given one hallmark indicator absent", t, func() {
		Convey("It should still reflect partial measurement alignment", func() {
			confidence := AlignConfidence(0.8, 0)

			So(confidence, ShouldBeGreaterThan, 0)
			So(confidence, ShouldBeLessThan, 1)
		})
	})
}

func TestConfidenceFromScore(t *testing.T) {
	Convey("Given a bounded measurement score", t, func() {
		Convey("It should map into (0, 1) without pinning at fifty percent", func() {
			confidence := ConfidenceFromScore(1)

			So(confidence, ShouldBeGreaterThan, 0.5)
			So(confidence, ShouldBeLessThan, 1)
		})
	})
}

func TestExcessRatio(t *testing.T) {
	Convey("Given a ratio above unity", t, func() {
		Convey("It should map excess into (0, 1)", func() {
			So(ExcessRatio(2), ShouldAlmostEqual, 0.5, 0.0001)
			So(ExcessRatio(1), ShouldEqual, 0)
		})
	})
}
