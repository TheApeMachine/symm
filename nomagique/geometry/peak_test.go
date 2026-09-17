package geometry_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/geometry"
)

func TestPeakNext(t *testing.T) {
	Convey("Peak assigns root basins across spanning forest edges", t, func() {
		nA := geometry.NewCoordinate(0, 0)
		nB := geometry.NewCoordinate(1, 0)
		nC := geometry.NewCoordinate(2, 0)

		// A (auth 1.0) -- B (auth 0.2) -- C (auth 0.9)
		// Two peaks: A and C. B climbs to A or C.
		eAB := geometry.NewEdge(nA, nB, geometry.NewWeight(1.0, 1.0))
		eAB.Authority = [2]float64{1.0, 0.2}

		eBC := geometry.NewEdge(nB, nC, geometry.NewWeight(1.0, 1.0))
		eBC.Authority = [2]float64{0.2, 0.9}

		peak := geometry.NewPeak()
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(eAB))
			yield(unsafe.Pointer(eBC))
		}

		var results []*geometry.Edge
		for out := range peak.Next(in) {
			results = append(results, (*geometry.Edge)(out))
		}

		So(peak.Error(), ShouldBeNil)
		So(len(results), ShouldEqual, 2)
		// One edge connects same basin (B climbed to A or C), the other connects differing basins (border!)
		So(results[0].Basin[0] != results[1].Basin[1], ShouldBeTrue)
	})
}
