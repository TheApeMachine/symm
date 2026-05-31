package ui

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
)

func TestDefaultDashboardLayout(t *testing.T) {
	convey.Convey("Given the default dashboard layout", t, func() {
		document := DefaultDashboardLayout(testNow())

		convey.Convey("It should include schema-driven surface and gauge panels", func() {
			convey.So(document.Event, convey.ShouldEqual, "layout")
			convey.So(len(document.Panels), convey.ShouldBeGreaterThanOrEqualTo, 4)

			wire := document.Wire()

			convey.So(wire["event"], convey.ShouldEqual, "layout")
			convey.So(wire["panels"], convey.ShouldNotBeNil)
		})
	})
}

func testNow() time.Time {
	return time.Unix(1_700_000_000, 0).UTC()
}
