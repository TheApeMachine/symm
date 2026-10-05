package calculus

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestAtanh(t *testing.T) {
	Convey("Atanh computes inverse hyperbolic tangent", t, func() {
		op := NewAtanh()
		state := data.NewState(data.NewMap("value", "value"))
		adapter := data.NewAdapter(nil, state)
		input := data.NewOutputMap()
		input.Values["value"] = 0.5

		for range adapter.Next(data.NewValue(input)) {
		}

		data.Read[*data.Adapter](op.Next(data.NewValue(adapter)))
		So(op.Error(), ShouldBeNil)
		So(op.output.Values["value"], ShouldAlmostEqual, math.Atanh(0.5), 1e-9)

		Convey("Out of range value produces domain error", func() {
			errOp := NewAtanh()
			errState := data.NewState(data.NewMap("value", "value"))
			errAdapter := data.NewAdapter(nil, errState)
			errInput := data.NewOutputMap()
			errInput.Values["value"] = 1.5

			for range errAdapter.Next(data.NewValue(errInput)) {
			}

			data.Read[*data.Adapter](errOp.Next(data.NewValue(errAdapter)))
			So(errOp.Error(), ShouldNotBeNil)
		})
	})
}
