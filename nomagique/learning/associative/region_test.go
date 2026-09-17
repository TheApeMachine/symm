package associative_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/learning/associative"
)

func TestRegionNext(t *testing.T) {
	Convey("Region extracts dominant basin signature via Argmax over authority", t, func() {
		nodeA := geometry.NewCoordinate(0, 0)
		nodeB := geometry.NewCoordinate(1, 0)
		nodeC := geometry.NewCoordinate(2, 0)

		edgeOne := geometry.NewEdge(nodeA, nodeB, geometry.NewWeight(1.0, 1.0))
		edgeOne.Basin = [2]int{0, 0}
		edgeOne.Authority = [2]float64{0.5, 0.5}

		edgeTwo := geometry.NewEdge(nodeB, nodeC, geometry.NewWeight(0.5, 1.0))
		edgeTwo.Basin = [2]int{0, 1}
		edgeTwo.Authority = [2]float64{0.5, 2.0}

		region := associative.NewRegion()
		in := func(yield func(unsafe.Pointer) bool) {
			if !yield(unsafe.Pointer(edgeOne)) {
				return
			}

			yield(unsafe.Pointer(edgeTwo))
		}

		var signatures [][]byte
		for out := range region.Next(in) {
			signatures = append(signatures, *(*[]byte)(out))
		}

		So(region.Error(), ShouldBeNil)
		So(len(signatures), ShouldEqual, 1)
		So(string(signatures[0]), ShouldEqual, "2,0")
	})

	Convey("basin ordinal changes but content signature remains equivalent", t, func() {
		nodeA := geometry.NewCoordinate(0, 0)
		nodeB := geometry.NewCoordinate(1, 0)
		nodeC := geometry.NewCoordinate(2, 0)

		edgeOne := geometry.NewEdge(nodeA, nodeB, geometry.NewWeight(1.0, 1.0))
		edgeOne.Basin = [2]int{4, 4}
		edgeOne.Authority = [2]float64{0.5, 0.5}

		edgeTwo := geometry.NewEdge(nodeB, nodeC, geometry.NewWeight(0.5, 1.0))
		edgeTwo.Basin = [2]int{4, 9}
		edgeTwo.Authority = [2]float64{0.5, 2.0}

		region := associative.NewRegion()
		in := func(yield func(unsafe.Pointer) bool) {
			if !yield(unsafe.Pointer(edgeOne)) {
				return
			}

			yield(unsafe.Pointer(edgeTwo))
		}

		var signatures [][]byte
		for out := range region.Next(in) {
			signatures = append(signatures, *(*[]byte)(out))
		}

		So(region.Error(), ShouldBeNil)
		So(len(signatures), ShouldEqual, 1)
		So(string(signatures[0]), ShouldEqual, "2,0")
	})
}
