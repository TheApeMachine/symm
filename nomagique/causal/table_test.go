package causal_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/causal"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestTableNext(t *testing.T) {
	Convey("Interventional expectation follows the affine structural model", t, func() {
		rows := [][]float64{
			{0, 0, 1},
			{1, 0, 2},
			{0, 1, 4},
			{1, 1, 5},
			{2, 0, 3},
			{2, 1, 6},
		}
		node := causal.NewTable(4, rows, 2, 1, []int{0}, true)
		So(node.Error(), ShouldBeNil)

		for _, level := range []float64{1, 0, 2, -1} {
			var results []float64

			for pointer := range node.Next(data.NewValue(level).Next(nil)) {
				results = append(results, *(*float64)(pointer))
			}

			So(node.Error(), ShouldBeNil)
			So(len(results), ShouldEqual, 2)
			So(results[0], ShouldAlmostEqual, 2+3*level)
			So(results[1], ShouldEqual, 1.0)
		}
	})

	Convey("The stump strategy standardizes over the same evidence", t, func() {
		rows := [][]float64{
			{0, 0, 1},
			{1, 0, 2},
			{0, 1, 4},
			{1, 1, 5},
			{2, 0, 3},
			{2, 1, 6},
		}
		node := causal.NewTable(4, rows, 2, 1, []int{0}, false)
		So(node.Error(), ShouldBeNil)

		var results []float64

		for pointer := range node.Next(data.NewValue(1.0).Next(nil)) {
			results = append(results, *(*float64)(pointer))
		}

		So(node.Error(), ShouldBeNil)
		So(len(results), ShouldEqual, 2)
		So(results[0], ShouldAlmostEqual, 2+3*1)
		So(results[1], ShouldEqual, 1.0)
	})

	Convey("Evidence short of the minimum is refused", t, func() {
		node := causal.NewTable(4, [][]float64{{0, 0, 1}, {1, 0, 2}}, 2, 1, []int{0}, true)
		So(node.Error(), ShouldNotBeNil)
	})
}

func TestTableAbductive(t *testing.T) {
	Convey("Counterfactuals retain factual noise on the affine model", t, func() {
		rows := [][]float64{
			{0, 0, 1},
			{1, 0, 2},
			{0, 1, 4},
			{1, 1, 5},
			{2, 0, 3},
			{2, 1, 6},
		}
		node := causal.NewTable(4, rows, 2, 1, []int{0}, true)
		So(node.Error(), ShouldBeNil)

		for _, level := range []float64{1, 0, 2, -1} {
			for _, noise := range []float64{0, 0.25, -0.5} {
				var results []float64

				for pointer := range node.Next(data.NewValue(level, 0.0, 0.0, 1.0+noise).Next(nil)) {
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

	Convey("A singular fit surfaces the non-identifiable state", t, func() {
		rows := [][]float64{
			{1, 0, 2},
			{1, 0, 3},
			{1, 1, 4},
			{1, 1, 5},
		}
		node := causal.NewTable(4, rows, 2, 1, []int{0}, true)
		So(node.Error(), ShouldNotBeNil)
	})
}
