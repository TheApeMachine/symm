package statistic_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/statistic"
)

func TestSympathyNext(t *testing.T) {
	Convey("Sympathy measures directional and relative magnitude attraction between metrics", t, func() {
		coordA := geometry.NewCoordinate(0, 0)
		coordB := geometry.NewCoordinate(1, 0)

		sympathy := statistic.NewSympathy[*geometry.Coordinate]()

		obsA := statistic.NewObservation(coordA, 0.4, 1.0, 1.0)
		obsB := statistic.NewObservation(coordB, 0.4, 0.8, 1.0)

		for range sympathy.Next(sequence.NewOne(unsafe.Pointer(obsA)).Next(nil)) {
		}

		var edges []*geometry.Edge
		for result := range sympathy.Next(sequence.NewOne(unsafe.Pointer(obsB)).Next(nil)) {
			edges = append(edges, (*geometry.Edge)(result))
		}

		So(sympathy.Error(), ShouldBeNil)
		So(len(edges), ShouldEqual, 1)
		So(edges[0].Left, ShouldEqual, coordA)
		So(edges[0].Right, ShouldEqual, coordB)

		var weightValues []float64
		for ptr := range edges[0].Next(nil) {
			weightValues = append(weightValues, *(*float64)(ptr))
		}

		So(len(weightValues), ShouldEqual, 2)
		So(weightValues[0], ShouldBeGreaterThan, 0) // Strength > 0
	})
}
