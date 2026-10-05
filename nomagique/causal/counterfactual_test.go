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
		node := causal.NewCounterfactual(1e-15)

		for _, level := range []float64{1, 0, 2, -1} {
			for _, noise := range []float64{0, 0.25, -0.5} {
				state := data.NewState(data.NewMap(
					"level", "level",
					"actual", "actual",
					"factual", "factual",
					"predicted", "predicted",
				))
				adapter := data.NewAdapter(nil, state)
				input := data.NewOutputMap()
				input.Values["level"] = level
				input.Values["actual"] = 1.0 + noise
				input.Values["factual"] = 1.0
				input.Values["predicted"] = 1.0 + 3.0*level

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
