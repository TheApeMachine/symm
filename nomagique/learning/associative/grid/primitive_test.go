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
		step1 := []float64{1.0, 0.5, 2.0, 1.0}
		tokens1 := data.Read[[]uint64](grid.Next(data.NewValue(step1)))
		So(len(tokens1), ShouldEqual, 2)
		So(tokens1[0], ShouldNotEqual, 0)
		So(tokens1[1], ShouldNotEqual, 0)
		So(tokens1[0], ShouldNotEqual, tokens1[1])

		// Step 2: second input with region 1 falling and region 2 rising
		step2 := []float64{0.2, 0.1, 3.0, 1.0}
		tokens2 := data.Read[[]uint64](grid.Next(data.NewValue(step2)))
		So(len(tokens2), ShouldEqual, 2)
		So(tokens2[0], ShouldNotEqual, 0)
		So(tokens2[1], ShouldNotEqual, 0)
	})

	Convey("Shape mismatch aborts pipeline yielding no output", t, func() {
		regions := 2
		channels := 4
		weights := make([]float64, 8)
		grid := NewPrimitive(regions, channels, weights)

		wrongInput := []float64{1.0, 2.0} // only 2 channels, expects 4
		tokens := data.Read[[]uint64](grid.Next(data.NewValue(wrongInput)))
		So(len(tokens), ShouldEqual, 0)
	})
}
