package grid

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestEmit(t *testing.T) {
	Convey("Given an Emit primitive configured for 3 regions", t, func() {
		emitter := NewEmit(3)

		Convey("Accumulates tokens and yields when matching region count", func() {
			state := data.NewState(
				data.NewMap("token_0", "token_0", "token_1", "token_1", "token_2", "token_2"),
			)
			adapter := data.NewAdapter(nil, state)
			inputValues := data.NewOutputMap()
			inputValues.Values["token_0"] = 101
			inputValues.Values["token_1"] = 102
			inputValues.Values["token_2"] = 103

			for range adapter.Next(data.NewValue(inputValues)) {
			}

			data.Read[*data.Adapter](emitter.Next(data.NewValue(adapter)))

			So(emitter.Error(), ShouldBeNil)
			So(emitter.output.Values["token_0"], ShouldEqual, 101)
			So(emitter.output.Values["token_1"], ShouldEqual, 102)
			So(emitter.output.Values["token_2"], ShouldEqual, 103)
		})
	})
}
