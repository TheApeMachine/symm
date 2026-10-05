package calculus

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestSquare(t *testing.T) {
	Convey("Square computes square of arrival", t, func() {
		op := NewSquare()
		state := data.NewState(data.NewMap("value", "value"))
		adapter := data.NewAdapter(nil, state)
		input := data.NewOutputMap()
		input.Values["value"] = 5.0

		for range adapter.Next(data.NewValue(input)) {
		}

		data.Read[*data.Adapter](op.Next(data.NewValue(adapter)))
		So(op.Error(), ShouldBeNil)
		So(op.output.Values["value"], ShouldEqual, 25.0)
	})
}
