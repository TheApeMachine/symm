package grid

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestExtract(t *testing.T) {
	Convey("Given an Extract primitive configured for 3 channels", t, func() {
		extractor := NewExtract(3)

		Convey("A matching input slice passes through directly", func() {
			state := data.NewState(
				data.NewMap("channel_0", "channel_0", "channel_1", "channel_1", "channel_2", "channel_2"),
			)
			adapter := data.NewAdapter(nil, state)
			inputValues := data.NewOutputMap()
			inputValues.Values["channel_0"] = 1.5
			inputValues.Values["channel_1"] = 2.5
			inputValues.Values["channel_2"] = -0.5

			for range adapter.Next(data.NewValue(inputValues)) {
			}

			data.Read[*data.Adapter](extractor.Next(data.NewValue(adapter)))
			So(extractor.Error(), ShouldBeNil)
			So(extractor.output.Values["channel_0"], ShouldEqual, 1.5)
			So(extractor.output.Values["channel_1"], ShouldEqual, 2.5)
			So(extractor.output.Values["channel_2"], ShouldEqual, -0.5)
		})

		Convey("A mismatching slice records ErrShape and yields nothing", func() {
			state := data.NewState(
				data.NewMap("channel_0", "channel_0", "channel_1", "channel_1"),
			)
			adapter := data.NewAdapter(nil, state)
			inputValues := data.NewOutputMap()
			inputValues.Values["channel_0"] = 1.5
			inputValues.Values["channel_1"] = 2.5

			for range adapter.Next(data.NewValue(inputValues)) {
			}

			res := data.Read[*data.Adapter](extractor.Next(data.NewValue(adapter)))
			So(res, ShouldBeNil)
			So(errors.Is(extractor.Error(), core.ErrShape), ShouldBeTrue)
		})
	})
}
