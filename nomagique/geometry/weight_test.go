package geometry_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/geometry"
)

func TestWeight(t *testing.T) {
	Convey("Weight yields strength when stepped with unit scalar", t, func() {
		weight := geometry.NewWeight(0.85, core.Unit)
		So(weight(core.Unit), ShouldEqual, 0.85)
	})

	Convey("Weight scales arriving scalars by strength", t, func() {
		weight := geometry.NewWeight(0.5, -core.Unit)
		So(weight(10.0), ShouldEqual, 5.0)
	})
}
