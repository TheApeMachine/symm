package calculus

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestAbsolute(t *testing.T) {
	Convey("Absolute returns non-negative magnitude", t, func() {
		op := NewAbsolute()
		state := data.NewState(data.NewMap("value", "value"))
		adapter := data.NewAdapter(nil, state)
		input := data.NewOutputMap()
		input.Values["value"] = -42.5

		for range adapter.Next(data.NewValue(input)) {
		}

		data.Read[*data.Adapter](op.Next(data.NewValue(adapter)))
		So(op.Error(), ShouldBeNil)
		So(op.output.Values["value"], ShouldEqual, 42.5)
		So(op.output.Values["absolute"], ShouldEqual, 42.5)
	})
}
