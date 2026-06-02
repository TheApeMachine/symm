package leadlag

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestLeadlagReading(t *testing.T) {
	Convey("Given a stalled anchor", t, func() {
		category, evidence := leadlagReading(false, 0.001, 0.5, 0)

		Convey("It should classify anchor stall", func() {
			So(category, ShouldEqual, perspectives.CategoryAnchorStall)
			So(evidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a stalled anchor with a moderately negative move", t, func() {
		category, evidence := leadlagReading(false, -5.37, 0, 0)

		Convey("It should keep stall evidence on the unit interval", func() {
			So(category, ShouldEqual, perspectives.CategoryAnchorStall)
			So(evidence, ShouldBeGreaterThan, 0)
			So(evidence, ShouldBeLessThanOrEqualTo, 1)
		})
	})

	Convey("Given a stalled anchor with a large negative move", t, func() {
		category, evidence := leadlagReading(false, -108.4, 0, 0)

		Convey("It should keep stall evidence on the unit interval", func() {
			So(category, ShouldEqual, perspectives.CategoryAnchorStall)
			So(evidence, ShouldBeGreaterThan, 0)
			So(evidence, ShouldBeLessThanOrEqualTo, 1)
		})
	})

	Convey("Given synchronized drift", t, func() {
		category, _ := leadlagReading(true, 0.05, 0.8, 0)

		Convey("It should classify synchronized drift", func() {
			So(category, ShouldEqual, perspectives.CategorySynchronizedDrift)
		})
	})
}
