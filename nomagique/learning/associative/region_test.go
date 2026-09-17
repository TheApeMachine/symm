package associative_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/learning/associative"
)

func TestRegionNext(t *testing.T) {
	Convey("Region extracts dominant basin token via Argmax over authority", t, func() {
		nA := geometry.NewCoordinate(0, 0)
		nB := geometry.NewCoordinate(1, 0)
		nC := geometry.NewCoordinate(2, 0)

		// Edge 1: Basin 0 (Authority 0.5) to Basin 0 (Authority 0.5) -> Basin 0 total = 1.0
		e1 := geometry.NewEdge(nA, nB, geometry.NewWeight(1.0, 1.0))
		e1.Basin = [2]int{0, 0}
		e1.Authority = [2]float64{0.5, 0.5}

		// Edge 2: Basin 0 (Authority 0.5) to Basin 1 (Authority 2.0) -> Basin 1 has authority 2.0
		e2 := geometry.NewEdge(nB, nC, geometry.NewWeight(0.5, 1.0))
		e2.Basin = [2]int{0, 1}
		e2.Authority = [2]float64{0.5, 2.0}

		// Total Basin 0: 0.5 + 0.5 + 0.5 = 1.5
		// Total Basin 1: 2.0
		// Winner: Basin 1 -> token "r1"

		region := associative.NewRegion()
		in := func(yield func(unsafe.Pointer) bool) {
			if !yield(unsafe.Pointer(e1)) {
				return
			}

			yield(unsafe.Pointer(e2))
		}

		var tokens [][]byte
		for out := range region.Next(in) {
			tokens = append(tokens, *(*[]byte)(out))
		}

		So(region.Error(), ShouldBeNil)
		So(len(tokens), ShouldEqual, 1)
		So(string(tokens[0]), ShouldEqual, "r1")
	})
}
