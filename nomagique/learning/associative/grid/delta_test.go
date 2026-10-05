package grid

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestDelta(t *testing.T) {
	Convey("Given a Delta primitive configured for 2 regions", t, func() {
		delta := NewDelta(2)

		Convey("Sequential steps compute levels and changes accurately", func() {
			state1 := data.NewState(
				data.NewMap("level_0", "level_0", "level_1", "level_1"),
			)
			adapter1 := data.NewAdapter(nil, state1)
			input1 := data.NewOutputMap()
			input1.Values["level_0"] = 10.0
			input1.Values["level_1"] = 20.0

			for range adapter1.Next(data.NewValue(input1)) {
			}

			data.Read[*data.Adapter](delta.Next(data.NewValue(adapter1)))
			So(delta.Error(), ShouldBeNil)
			// Region 1: ID=1, level=10, change=10-0=10
			So(delta.output.Values["level_0"], ShouldEqual, 10.0)
			So(delta.output.Values["change_0"], ShouldEqual, 10.0)
			So(delta.output.Values["region_0"], ShouldEqual, 1.0)
			// Region 2: ID=2, level=20, change=20-0=20
			So(delta.output.Values["level_1"], ShouldEqual, 20.0)
			So(delta.output.Values["change_1"], ShouldEqual, 20.0)
			So(delta.output.Values["region_1"], ShouldEqual, 2.0)

			state2 := data.NewState(
				data.NewMap("level_0", "level_0", "level_1", "level_1"),
			)
			adapter2 := data.NewAdapter(nil, state2)
			input2 := data.NewOutputMap()
			input2.Values["level_0"] = 12.0
			input2.Values["level_1"] = 15.0

			for range adapter2.Next(data.NewValue(input2)) {
			}

			data.Read[*data.Adapter](delta.Next(data.NewValue(adapter2)))
			So(delta.Error(), ShouldBeNil)
			// Region 1: ID=1, level=12, change=12-10=2
			So(delta.output.Values["level_0"], ShouldEqual, 12.0)
			So(delta.output.Values["change_0"], ShouldEqual, 2.0)
			// Region 2: ID=2, level=15, change=15-20=-5
			So(delta.output.Values["level_1"], ShouldEqual, 15.0)
			So(delta.output.Values["change_1"], ShouldEqual, -5.0)
		})

		Convey("Shape mismatch records ErrShape", func() {
			state := data.NewState(
				data.NewMap("level_0", "level_0"),
			)
			adapter := data.NewAdapter(nil, state)
			input := data.NewOutputMap()
			input.Values["level_0"] = 1.0

			for range adapter.Next(data.NewValue(input)) {
			}

			data.Read[*data.Adapter](delta.Next(data.NewValue(adapter)))
			So(errors.Is(delta.Error(), core.ErrShape), ShouldBeTrue)
		})
	})
}
