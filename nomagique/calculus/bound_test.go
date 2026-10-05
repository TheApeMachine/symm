package calculus

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestBound(t *testing.T) {
	Convey("Bound clamps value between lower and upper", t, func() {
		op := NewBound()
		state := data.NewState(
			data.NewMap("value", "value", "lower", "lower", "upper", "upper"),
		)
		adapter := data.NewAdapter(nil, state)
		input := data.NewOutputMap()
		input.Values["lower"] = 0.0
		input.Values["upper"] = 10.0

		Convey("Value below lower clamps to lower", func() {
			input.Values["value"] = -5.0
			for range adapter.Next(data.NewValue(input)) {
			}
			data.Read[*data.Adapter](op.Next(data.NewValue(adapter)))
			So(op.Error(), ShouldBeNil)
			So(op.output.Values["value"], ShouldEqual, 0.0)
		})

		Convey("Value above upper clamps to upper", func() {
			input.Values["value"] = 15.0
			for range adapter.Next(data.NewValue(input)) {
			}
			data.Read[*data.Adapter](op.Next(data.NewValue(adapter)))
			So(op.Error(), ShouldBeNil)
			So(op.output.Values["value"], ShouldEqual, 10.0)
		})

		Convey("Value within bounds is preserved", func() {
			input.Values["value"] = 5.0
			for range adapter.Next(data.NewValue(input)) {
			}
			data.Read[*data.Adapter](op.Next(data.NewValue(adapter)))
			So(op.Error(), ShouldBeNil)
			So(op.output.Values["value"], ShouldEqual, 5.0)
		})
	})
}
