package leadlag

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestLeadlagReading(t *testing.T) {
	Convey("Given a stalled anchor with unit stall margin", t, func() {
		category, clarity, strength := leadlagReading(false, 0.8, 0, 0)

		Convey("It should classify anchor stall with no phenomenon strength", func() {
			So(category, ShouldEqual, types.CategoryAnchorStall)
			So(clarity, ShouldEqual, 0.8)
			So(strength, ShouldEqual, 0) // a stall is the absence of a lead-lag signal
		})
	})

	Convey("Given a stalled anchor with zero margin", t, func() {
		category, clarity, strength := leadlagReading(false, 0, 0, 0)

		Convey("It should emit zero stall clarity and strength", func() {
			So(category, ShouldEqual, types.CategoryAnchorStall)
			So(clarity, ShouldEqual, 0)
			So(strength, ShouldEqual, 0)
		})
	})

	Convey("Given synchronized drift", t, func() {
		category, clarity, strength := leadlagReading(true, 0, 0.8, 0)

		Convey("It should classify synchronized drift, strength = the correlation", func() {
			So(category, ShouldEqual, types.CategorySynchronizedDrift)
			So(strength, ShouldEqual, 0.8)        // standout carries the correlation magnitude, not the threshold margin
			So(clarity, ShouldNotEqual, strength) // clarity (boundary margin) is a different quantity
		})
	})

	Convey("Given inverse contemporaneous movement", t, func() {
		category, clarity, strength := leadlagReading(true, 0, -0.9, 0)

		Convey("It should classify decoupling with unit-band clarity", func() {
			So(category, ShouldEqual, types.CategoryDecoupledMove)
			So(clarity, ShouldBeGreaterThan, 0)
			So(clarity, ShouldBeLessThanOrEqualTo, 1)
			So(strength, ShouldEqual, 0)
		})
	})
}
