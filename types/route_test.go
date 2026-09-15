package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRoute(t *testing.T) {
	Convey("Given route tracking and filtering", t, func() {
		original := Route()
		Reset(func() {
			SetRoute(original)
		})

		Convey("Defaults to dashboard", func() {
			SetRoute("")
			So(Route(), ShouldEqual, "dashboard")
			So(AllowsRoute("cvd"), ShouldBeTrue)
			So(AllowsRoute("training"), ShouldBeTrue)
		})

		Convey("When on fluid route", func() {
			SetRoute("/fluid")
			So(Route(), ShouldEqual, "fluid")
			So(AllowsRoute("cvd"), ShouldBeFalse)
			So(AllowsRoute("hawkes"), ShouldBeFalse)
			So(AllowsRoute("depthflow"), ShouldBeFalse)
			So(AllowsRoute("training"), ShouldBeTrue)
			So(AllowsRoute(""), ShouldBeTrue)
		})

		Convey("When on learning route", func() {
			SetRoute("learning")
			So(Route(), ShouldEqual, "learning")
			So(AllowsRoute("cvd"), ShouldBeFalse)
			So(AllowsRoute("category"), ShouldBeTrue)
			So(AllowsRoute("cognition"), ShouldBeTrue)
			So(AllowsRoute("resonance"), ShouldBeTrue)
			So(AllowsRoute("training"), ShouldBeTrue)
		})
	})
}
