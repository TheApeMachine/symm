package engine

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestDirection(t *testing.T) {
	convey.Convey("Given a buy-side measurement type", t, func() {
		convey.Convey("It should return positive direction", func() {
			convey.So(Momentum.Direction(), convey.ShouldEqual, 1)
		})
	})

	convey.Convey("Given a sell-side measurement type", t, func() {
		convey.Convey("It should return negative direction", func() {
			convey.So(Dump.Direction(), convey.ShouldEqual, -1)
		})
	})
}
