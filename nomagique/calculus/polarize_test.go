package calculus

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestPolarize(t *testing.T) {
	Convey("Polarize splits signed values into nonnegative components", t, func() {
		op := NewPolarize()

		// Case 1: Value = 2.0, Scale = 2.0
		state1 := data.NewState(data.NewMap("value", "value", "scale", "scale"))
		adapter1 := data.NewAdapter(nil, state1)
		input1 := data.NewOutputMap()
		input1.Values["value"] = 2.0
		input1.Values["scale"] = 2.0

		for range adapter1.Next(data.NewValue(input1)) {
		}

		data.Read[*data.Adapter](op.Next(data.NewValue(adapter1)))
		So(op.Error(), ShouldBeNil)
		So(op.output.Values["alpha"], ShouldEqual, 2.0)
		So(op.output.Values["beta"], ShouldEqual, 0.0)
		So(op.output.Values["alpha_normalized"], ShouldEqual, 0.5)
		So(op.output.Values["value"], ShouldEqual, 0.5)

		// Case 2: Value = -2.0, Scale = 2.0
		state2 := data.NewState(data.NewMap("value", "value", "scale", "scale"))
		adapter2 := data.NewAdapter(nil, state2)
		input2 := data.NewOutputMap()
		input2.Values["value"] = -2.0
		input2.Values["scale"] = 2.0

		for range adapter2.Next(data.NewValue(input2)) {
		}

		data.Read[*data.Adapter](op.Next(data.NewValue(adapter2)))
		So(op.Error(), ShouldBeNil)
		So(op.output.Values["alpha"], ShouldEqual, 0.0)
		So(op.output.Values["beta"], ShouldEqual, 2.0)
		So(op.output.Values["beta_normalized"], ShouldEqual, 0.5)
		So(op.output.Values["value"], ShouldEqual, -0.5)
	})
}
