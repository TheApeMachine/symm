package core_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
)

func TestAction(t *testing.T) {
	Convey("Action constants represent canonical operations", t, func() {
		So(core.None, ShouldEqual, core.Action(0))
		So(core.Identify, ShouldEqual, core.Action(1))
		So(core.Read, ShouldEqual, core.Action(2))
		So(core.Write, ShouldEqual, core.Action(3))
		So(core.Execute, ShouldEqual, core.Action(4))
	})
}
