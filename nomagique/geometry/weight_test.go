package geometry_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestWeight(t *testing.T) {
	Convey("Weight yields strength when stepped with unit scalar", t, func() {
		weight := geometry.NewWeight(types.Const(0.85), types.Const(core.Unit))
		So(weight(core.Unit), ShouldEqual, 0.85)
	})

	Convey("Weight scales arriving scalars by strength", t, func() {
		weight := geometry.NewWeight(types.Const(0.5), types.Const(-core.Unit))
		So(weight(10.0), ShouldEqual, 5.0)
	})
}
