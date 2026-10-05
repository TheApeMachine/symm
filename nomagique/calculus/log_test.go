package calculus

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestLog(t *testing.T) {
	Convey("Log computes natural logarithm", t, func() {
		op := NewLog()
		state := data.NewState(data.NewMap("value", "value"))
		adapter := data.NewAdapter(nil, state)
		input := data.NewOutputMap()
		input.Values["value"] = math.E

		for range adapter.Next(data.NewValue(input)) {
		}

		data.Read[*data.Adapter](op.Next(data.NewValue(adapter)))
		So(op.Error(), ShouldBeNil)
		So(op.output.Values["value"], ShouldAlmostEqual, 1.0, 1e-9)

		Convey("Non-positive value records domain error", func() {
			errOp := NewLog()
			errState := data.NewState(data.NewMap("value", "value"))
			errAdapter := data.NewAdapter(nil, errState)
			errInput := data.NewOutputMap()
			errInput.Values["value"] = -1.0

			for range errAdapter.Next(data.NewValue(errInput)) {
			}

			data.Read[*data.Adapter](errOp.Next(data.NewValue(errAdapter)))
			So(errOp.Error(), ShouldNotBeNil)
		})
	})
}
