package calculus

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestSecondDifference(t *testing.T) {
	Convey("SecondDifference computes 2*center - left - right", t, func() {
		op := NewSecondDifference()
		state := data.NewState(data.NewMap("center", "center", "left", "left", "right", "right"))
		adapter := data.NewAdapter(nil, state)
		input := data.NewOutputMap()
		input.Values["center"] = 5.0
		input.Values["left"] = 3.0
		input.Values["right"] = 4.0

		for range adapter.Next(data.NewValue(input)) {
		}

		data.Read[*data.Adapter](op.Next(data.NewValue(adapter)))
		So(op.Error(), ShouldBeNil)
		// 2*5 - 3 - 4 = 10 - 7 = 3
		So(op.output.Values["value"], ShouldEqual, 3.0)
	})
}
