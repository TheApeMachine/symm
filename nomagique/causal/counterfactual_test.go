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
				actual := 1.0 + noise
				factual := 1.0
				predicted := 1.0 + 3.0*level

				var results []float64

				for pointer := range node.Next(data.NewValue(level, actual, factual, predicted).Next(nil)) {
					results = append(results, *(*float64)(pointer))
				}

				So(node.Error(), ShouldBeNil)
				So(len(results), ShouldEqual, 4)
				So(results[0], ShouldAlmostEqual, 1+3*level+noise)
				So(results[1], ShouldAlmostEqual, noise)
				So(results[2], ShouldAlmostEqual, 1/(1+math.Abs(noise)))
				So(results[3], ShouldEqual, 1.0)
			}
		}
	})
}
