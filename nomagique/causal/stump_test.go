package causal_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/causal"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestStumpNext(t *testing.T) {
	Convey("The stump strategy standardizes over the evidence", t, func() {
		rows := [][]float64{
			{0, 0, 1},
			{1, 0, 2},
			{0, 1, 4},
			{1, 1, 5},
			{2, 0, 3},
			{2, 1, 6},
		}
		node := causal.NewStump(rows, 2, 1, []int{0})
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
}
