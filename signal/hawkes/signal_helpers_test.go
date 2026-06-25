package hawkes

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTailQuantile(t *testing.T) {
	Convey("Given arrival gap count and branching ratio", t, func() {
		Convey("It should derive rank from sample size without hard clamps", func() {
			So(tailQuantile(2, 0), ShouldAlmostEqual, 0.25, 0.0001)
			So(tailQuantile(10, 1), ShouldAlmostEqual, 0.9, 0.0001)
			So(tailQuantile(0, 0.6), ShouldEqual, 0.6)
		})
	})
}

func TestStabilityCeiling(t *testing.T) {
	Convey("Given branching ratio and gap evidence", t, func() {
		Convey("It should scale toward unity with clustering and evidence", func() {
			So(stabilityCeiling(0, 5), ShouldAlmostEqual, 5.0/6.0, 0.0001)
			So(stabilityCeiling(1, 5), ShouldEqual, 1)
			So(stabilityCeiling(0.8, 10), ShouldBeGreaterThan, 0.8)
			So(stabilityCeiling(0.8, 10), ShouldBeLessThan, 1)
		})
	})
}

func TestCriticalRadiusCap(t *testing.T) {
	Convey("Given clustered arrivals", t, func() {
		stamps := []float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		branching := branchingRatio(stamps)

		Convey("It should cap radius below the branching-derived ceiling", func() {
			cap := criticalRadiusCap(stamps, branching)

			So(cap, ShouldBeLessThanOrEqualTo, stabilityCeiling(branching, len(interArrivalGaps(stamps))))
			So(cap, ShouldBeGreaterThanOrEqualTo, branching)
		})
	})
}
