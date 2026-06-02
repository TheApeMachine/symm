package leadlag

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestLeadlagReading(t *testing.T) {
	Convey("Given a stalled anchor with unit stall margin", t, func() {
		category, evidence := leadlagReading(false, 0.8, 0, 0)

		Convey("It should classify anchor stall", func() {
			So(category, ShouldEqual, perspectives.CategoryAnchorStall)
			So(evidence, ShouldEqual, 0.8)
		})
	})

	Convey("Given a stalled anchor with zero margin", t, func() {
		category, evidence := leadlagReading(false, 0, 0, 0)

		Convey("It should emit zero stall evidence", func() {
			So(category, ShouldEqual, perspectives.CategoryAnchorStall)
			So(evidence, ShouldEqual, 0)
		})
	})

	Convey("Given synchronized drift", t, func() {
		category, _ := leadlagReading(true, 0, 0.8, 0)

		Convey("It should classify synchronized drift", func() {
			So(category, ShouldEqual, perspectives.CategorySynchronizedDrift)
		})
	})
}
