package causal_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/causal"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestBackdoorNext(t *testing.T) {
	Convey("Interventional expectation follows the affine structural model", t, func() {
		node := causal.NewBackdoor(1e-15)
		rows := [][]float64{{0, 0, 1}, {1, 0, 2}, {0, 1, 4}, {1, 1, 5}, {2, 0, 3}, {2, 1, 6}}

		for _, level := range []float64{1, 0, 2, -1} {
			got, err := transport.Evaluate(node, transport.Values(causal.Query{
				Rows: rows, Features: []int{0, 1}, Target: 2, Treatment: 1, Level: level,
			}))
			So(err, ShouldBeNil)
			So(got.Defined, ShouldBeTrue)
			So(got.Expectation, ShouldAlmostEqual, 2+3*level)
		}
	})
}

func TestCounterfactualNext(t *testing.T) {
	Convey("Counterfactuals retain factual noise on the affine model", t, func() {
		node := causal.NewCounterfactual(1e-15)
		rows := [][]float64{{0, 0, 1}, {1, 0, 2}, {0, 1, 4}, {1, 1, 5}, {2, 0, 3}, {2, 1, 6}}

		for _, level := range []float64{1, 0, 2, -1} {
			for _, noise := range []float64{0, 0.25, -0.5} {
				got, err := transport.Evaluate(node, transport.Values(causal.Query{
					Rows: rows, Features: []int{0, 1}, Target: 2, Treatment: 1, Level: level,
					Actual: []float64{0, 0, 1 + noise},
				}))
				So(err, ShouldBeNil)
				So(got.Defined, ShouldBeTrue)
				So(got.Noise, ShouldAlmostEqual, noise)
				So(got.Counterfactual, ShouldAlmostEqual, 1+3*level+noise)
			}
		}
	})
}

func TestCounterfactualRankDeficiency(t *testing.T) {
	Convey("A singular fit does not invent a counterfactual", t, func() {
		node := causal.NewCounterfactual(1e-15)
		got, err := transport.Evaluate(node, transport.Values(causal.Query{
			Rows:     [][]float64{{1, 0, 2}, {1, 0, 3}, {1, 1, 4}, {1, 1, 5}},
			Features: []int{0, 1}, Target: 2, Treatment: 1, Level: 1,
			Actual: []float64{1, 0, 2},
		}))
		So(err, ShouldBeNil)
		So(got.Defined, ShouldBeFalse)
		So(math.IsNaN(got.Counterfactual), ShouldBeTrue)
	})
}
