package geometry_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/geometry"
)

func TestRelaxationNext(t *testing.T) {
	Convey("Relaxation shifts virtual coordinates under attractive force", t, func() {
		coordA := geometry.NewCoordinate(0, 0)
		coordB := geometry.NewCoordinate(10, 0)
		weight := geometry.NewWeight(1.0, 1.0)
		edge := geometry.NewEdge(coordA, coordB, weight)
		edge.Distance = 2.0 // Target distance is 2.0, current is 10.0 -> should attract

		relaxation := geometry.NewRelaxation()
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(edge))
		}

		var results []*geometry.Edge
		for out := range relaxation.Next(in) {
			results = append(results, (*geometry.Edge)(out))
		}

		So(relaxation.Error(), ShouldBeNil)
		So(len(results), ShouldEqual, 1)
		// Store coordinates remain completely unmutated!
		So(coordA.X, ShouldEqual, 0)
		So(coordA.Y, ShouldEqual, 0)
		So(coordB.X, ShouldEqual, 10)
		So(coordB.Y, ShouldEqual, 0)
	})
}
