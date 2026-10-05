package calculus

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestRelativeChange(t *testing.T) {
	Convey("RelativeChange computes (current - previous) / previous", t, func() {
		op := NewRelativeChange()
		state := data.NewState(data.NewMap("current", "current", "previous", "previous"))
		adapter := data.NewAdapter(nil, state)
		input := data.NewOutputMap()
		input.Values["current"] = 120.0
		input.Values["previous"] = 100.0

		for range adapter.Next(data.NewValue(input)) {
		}

		data.Read[*data.Adapter](op.Next(data.NewValue(adapter)))
		So(op.Error(), ShouldBeNil)
		So(op.output.Values["value"], ShouldEqual, 0.2)

		Convey("Zero previous produces domain error", func() {
			errOp := NewRelativeChange()
			errState := data.NewState(data.NewMap("current", "current", "previous", "previous"))
			errAdapter := data.NewAdapter(nil, errState)
			errInput := data.NewOutputMap()
			errInput.Values["current"] = 100.0
			errInput.Values["previous"] = 0.0

			for range errAdapter.Next(data.NewValue(errInput)) {
			}

			data.Read[*data.Adapter](errOp.Next(data.NewValue(errAdapter)))
			So(errOp.Error(), ShouldNotBeNil)
		})
	})
}
