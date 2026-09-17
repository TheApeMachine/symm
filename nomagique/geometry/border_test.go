package geometry_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/geometry"
)

func TestBorderNext(t *testing.T) {
	Convey("Border yields edges that connect differing basins", t, func() {
		nA := geometry.NewCoordinate(0, 0)
		nB := geometry.NewCoordinate(1, 0)
		nC := geometry.NewCoordinate(10, 0)

		eInternal := geometry.NewEdge(nA, nB, geometry.NewWeight(1.0, 1.0))
		eInternal.Basin = [2]int{0, 0}

		eBorder := geometry.NewEdge(nB, nC, geometry.NewWeight(0.1, -1.0))
		eBorder.Basin = [2]int{0, 1}

		border := geometry.NewBorder()
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(eInternal))
			yield(unsafe.Pointer(eBorder))
		}

		var results []*geometry.Edge
		for out := range border.Next(in) {
			results = append(results, (*geometry.Edge)(out))
		}

		So(border.Error(), ShouldBeNil)
		So(len(results), ShouldEqual, 2)
		So(results[0].Border, ShouldBeFalse)
		So(results[1].Border, ShouldBeTrue)
	})
}
