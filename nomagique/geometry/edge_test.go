package geometry_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/geometry"
)

func TestEdge(t *testing.T) {
	Convey("Edge connects two identifiable coordinates across a weight primitive", t, func() {
		coordA := geometry.NewCoordinate(1, 2)
		coordB := geometry.NewCoordinate(3, 4)
		weight := geometry.NewWeight(0.75, 1.0)
		edge := geometry.NewEdge(coordA, coordB, weight)

		So(edge.Left, ShouldEqual, coordA)
		So(edge.Right, ShouldEqual, coordB)

		var values []float64
		for ptr := range edge.Next(nil) {
			values = append(values, *(*float64)(ptr))
		}

		So(len(values), ShouldEqual, 2)
		So(values[0], ShouldEqual, 0.75)
		So(values[1], ShouldEqual, 1.0)
	})
}
