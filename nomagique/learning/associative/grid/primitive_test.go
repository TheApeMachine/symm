package grid

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/tests"
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
		out1 := tests.CollectSeq[float64](grid.Next(data.NewValue(1.0, 0.5, 2.0, 1.0).Next(nil)))
		So(grid.Error(), ShouldBeNil)
		// Yields token_0, level_0, change_0, token_1, level_1, change_1
		So(len(out1), ShouldEqual, 6)
		token1_0 := out1[0]
		token1_1 := out1[3]
		So(token1_0, ShouldNotEqual, 0)
		So(token1_1, ShouldNotEqual, 0)
		So(token1_0, ShouldNotEqual, token1_1)

		// Step 2: second input with region 1 falling and region 2 rising
		out2 := tests.CollectSeq[float64](grid.Next(data.NewValue(0.2, 0.1, 3.0, 1.0).Next(nil)))
		So(grid.Error(), ShouldBeNil)
		token2_0 := out2[0]
		token2_1 := out2[3]
		So(token2_0, ShouldNotEqual, 0)
		So(token2_1, ShouldNotEqual, 0)
	})

	Convey("Shape mismatch aborts pipeline yielding no output", t, func() {
		regions := 2
		channels := 4
		weights := make([]float64, 8)
		grid := NewPrimitive(regions, channels, weights)

		res := tests.CollectSeq[float64](grid.Next(data.NewValue(1.0, 2.0).Next(nil)))
		So(len(res), ShouldEqual, 0)
		So(grid.Error(), ShouldNotBeNil)
	})
}
