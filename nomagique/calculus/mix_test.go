package calculus

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestMix(t *testing.T) {
	Convey("Mix linearly interpolates between left and right", t, func() {
		op := NewMix()
		state := data.NewState(
			data.NewMap("left", "left", "right", "right", "weight", "weight"),
		)
		adapter := data.NewAdapter(nil, state)
		input := data.NewOutputMap()
		input.Values["left"] = 10.0
		input.Values["right"] = 20.0
		input.Values["weight"] = 0.25

		for range adapter.Next(data.NewValue(input)) {
		}

		data.Read[*data.Adapter](op.Next(data.NewValue(adapter)))
		So(op.Error(), ShouldBeNil)
		So(op.output.Values["value"], ShouldEqual, 12.5)
	})
}
