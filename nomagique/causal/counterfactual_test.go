package causal_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/causal"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestCounterfactualNext(t *testing.T) {
	Convey("Counterfactuals retain factual noise on the affine model", t, func() {
		rows := [][]float64{
			{0, 0, 1},
			{1, 0, 2},
			{0, 1, 4},
			{1, 1, 5},
			{2, 0, 3},
			{2, 1, 6},
		}
		node := causal.NewCounterfactual(1e-15, rows, 2, 1, []int{0, 1})
		So(node.Error(), ShouldBeNil)

		for _, level := range []float64{1, 0, 2, -1} {
			for _, noise := range []float64{0, 0.25, -0.5} {
				state := data.NewState(data.NewMap(
					"level", "level",
					"actual_0", "actual_0",
					"actual_1", "actual_1",
					"actual_2", "actual_2",
				))
				adapter := data.NewAdapter(nil, state)
				input := data.NewOutputMap()
				input.Values["level"] = level
				input.Values["actual_0"] = 0
				input.Values["actual_1"] = 0
				input.Values["actual_2"] = 1 + noise

				for range adapter.Next(data.NewValue(input)) {
				}

				data.Read[*data.Adapter](node.Next(data.NewValue(adapter)))
				So(node.Error(), ShouldBeNil)

				outputState := data.NewMap(
					"counterfactual", "counterfactual",
					"noise", "noise",
					"precision", "precision",
					"defined", "defined",
				)
				var result data.Map[float64]

				for pointer := range adapter.Next(data.NewValue(outputState)) {
					result = *(*data.Map[float64])(pointer)
				}

				So(result.Values["defined"], ShouldEqual, 1.0)
				So(result.Values["noise"], ShouldAlmostEqual, noise)
				So(result.Values["counterfactual"], ShouldAlmostEqual, 1+3*level+noise)
				So(result.Values["precision"], ShouldAlmostEqual, 1/(1+math.Abs(noise)))
			}
		}
	})
}

func TestCounterfactualRankDeficiency(t *testing.T) {
	Convey("A singular fit surfaces error and does not invent a counterfactual", t, func() {
		rows := [][]float64{
			{1, 0, 2},
			{1, 0, 3},
			{1, 1, 4},
			{1, 1, 5},
		}
		node := causal.NewCounterfactual(1e-15, rows, 2, 1, []int{0, 1})
		So(node.Error(), ShouldNotBeNil)
	})
}
