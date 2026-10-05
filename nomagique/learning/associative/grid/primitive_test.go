package grid

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestGridPrimitive(t *testing.T) {
	Convey("Associative grid pipeline processes channels into region tokens", t, func() {
		regions := 2
		channels := 4
		// Region 1: observes channels 0 and 1
		// Region 2: observes channels 2 and 3
		weights := []float64{
			1.0, 1.0, 0.0, 0.0,
			0.0, 0.0, 1.0, -1.0,
		}

		grid := NewPrimitive(regions, channels, weights)

		// Step 1: initial input
		state1 := data.NewState(
			data.NewMap(
				"channel_0", "channel_0",
				"channel_1", "channel_1",
				"channel_2", "channel_2",
				"channel_3", "channel_3",
			),
		)
		adapter1 := data.NewAdapter(nil, state1)
		input1 := data.NewOutputMap()
		input1.Values["channel_0"] = 1.0
		input1.Values["channel_1"] = 0.5
		input1.Values["channel_2"] = 2.0
		input1.Values["channel_3"] = 1.0

		for range adapter1.Next(data.NewValue(input1)) {
		}

		data.Read[*data.Adapter](grid.Next(data.NewValue(adapter1)))
		So(grid.Error(), ShouldBeNil)
		token1_0 := grid.output.Values["token_0"]
		token1_1 := grid.output.Values["token_1"]
		So(token1_0, ShouldNotEqual, 0)
		So(token1_1, ShouldNotEqual, 0)
		So(token1_0, ShouldNotEqual, token1_1)

		// Step 2: second input with region 1 falling and region 2 rising
		state2 := data.NewState(
			data.NewMap(
				"channel_0", "channel_0",
				"channel_1", "channel_1",
				"channel_2", "channel_2",
				"channel_3", "channel_3",
			),
		)
		adapter2 := data.NewAdapter(nil, state2)
		input2 := data.NewOutputMap()
		input2.Values["channel_0"] = 0.2
		input2.Values["channel_1"] = 0.1
		input2.Values["channel_2"] = 3.0
		input2.Values["channel_3"] = 1.0

		for range adapter2.Next(data.NewValue(input2)) {
		}

		data.Read[*data.Adapter](grid.Next(data.NewValue(adapter2)))
		So(grid.Error(), ShouldBeNil)
		token2_0 := grid.output.Values["token_0"]
		token2_1 := grid.output.Values["token_1"]
		So(token2_0, ShouldNotEqual, 0)
		So(token2_1, ShouldNotEqual, 0)
	})

	Convey("Shape mismatch aborts pipeline yielding no output", t, func() {
		regions := 2
		channels := 4
		weights := make([]float64, 8)
		grid := NewPrimitive(regions, channels, weights)

		state := data.NewState(
			data.NewMap(
				"channel_0", "channel_0",
				"channel_1", "channel_1",
			),
		)
		adapter := data.NewAdapter(nil, state)
		input := data.NewOutputMap()
		input.Values["channel_0"] = 1.0
		input.Values["channel_1"] = 2.0

		for range adapter.Next(data.NewValue(input)) {
		}

		res := data.Read[*data.Adapter](grid.Next(data.NewValue(adapter)))
		So(res, ShouldBeNil)
		So(grid.Error(), ShouldNotBeNil)
	})
}
