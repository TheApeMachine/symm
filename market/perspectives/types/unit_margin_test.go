package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestUnitCompetitionMargin(t *testing.T) {
	Convey("Given a positive margin and scale", t, func() {
		margin := UnitCompetitionMargin(3, 1)

		Convey("It should stay on the unit interval without hard capping", func() {
			So(margin, ShouldAlmostEqual, 0.75, 0.0001)
			So(margin, ShouldBeLessThan, 1)
		})
	})

	Convey("Given pathological magnitude", t, func() {
		margin := UnitMagnitudeMargin(131_996_665_001.92592)

		Convey("It should saturate toward 1", func() {
			So(margin, ShouldBeGreaterThan, 0)
			So(margin, ShouldBeLessThanOrEqualTo, 1)
		})
	})
}
