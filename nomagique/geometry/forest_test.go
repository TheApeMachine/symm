package geometry_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/geometry"
)

func TestForestNext(t *testing.T) {
	Convey("Forest selects minimum spanning tree edges across nodes", t, func() {
		nA := geometry.NewCoordinate(0, 0)
		nB := geometry.NewCoordinate(1, 0)
		nC := geometry.NewCoordinate(2, 0)

		// Triangle graph: AB (dist 1), BC (dist 1), AC (dist 5)
		eAB := geometry.NewEdge(nA, nB, geometry.NewWeight(1.0, 1.0))
		eAB.Distance = 1.0

		eBC := geometry.NewEdge(nB, nC, geometry.NewWeight(1.0, 1.0))
		eBC.Distance = 1.0

		eAC := geometry.NewEdge(nA, nC, geometry.NewWeight(0.2, 1.0))
		eAC.Distance = 5.0

		forest := geometry.NewForest()
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(eAC))
			yield(unsafe.Pointer(eAB))
			yield(unsafe.Pointer(eBC))
		}

		var spanning []*geometry.Edge
		for out := range forest.Next(in) {
			spanning = append(spanning, (*geometry.Edge)(out))
		}

		So(forest.Error(), ShouldBeNil)
		So(len(spanning), ShouldEqual, 2)
		So(spanning[0], ShouldEqual, eAB)
		So(spanning[1], ShouldEqual, eBC)
	})
}
