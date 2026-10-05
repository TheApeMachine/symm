package calculus

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestReciprocal(t *testing.T) {
	Convey("Reciprocal computes multiplicative inverse", t, func() {
		op := NewReciprocal()
		state := data.NewState(data.NewMap("value", "value"))
		adapter := data.NewAdapter(nil, state)
		input := data.NewOutputMap()
		input.Values["value"] = 4.0

		for range adapter.Next(data.NewValue(input)) {
		}

		data.Read[*data.Adapter](op.Next(data.NewValue(adapter)))
		So(op.Error(), ShouldBeNil)
		So(op.output.Values["value"], ShouldEqual, 0.25)

		Convey("Zero value produces domain error", func() {
			errOp := NewReciprocal()
			errState := data.NewState(data.NewMap("value", "value"))
			errAdapter := data.NewAdapter(nil, errState)
			errInput := data.NewOutputMap()
			errInput.Values["value"] = 0.0

			for range errAdapter.Next(data.NewValue(errInput)) {
			}

			data.Read[*data.Adapter](errOp.Next(data.NewValue(errAdapter)))
			So(errOp.Error(), ShouldNotBeNil)
		})
	})
}
