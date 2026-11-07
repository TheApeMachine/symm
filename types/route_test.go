package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRoute(t *testing.T) {
	Convey("Route tracks the active dashboard surface", t, func() {
		SetRoute("dashboard")
		So(Route(), ShouldEqual, "dashboard")
		So(WantsGrid(), ShouldBeTrue)

		SetRoute("/learning")
		So(Route(), ShouldEqual, "learning")
		So(WantsGrid(), ShouldBeTrue)

		SetRoute("longtail")
		So(Route(), ShouldEqual, "longtail")
		So(WantsGrid(), ShouldBeFalse)

		SetRoute("workbench")
		So(WantsGrid(), ShouldBeFalse)

		SetRoute("")
		So(Route(), ShouldEqual, "dashboard")
	})
}
