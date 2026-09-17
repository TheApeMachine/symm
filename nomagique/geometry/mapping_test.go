package geometry_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/geometry"
)

func TestMappingNext(t *testing.T) {
	Convey("Mapping drives wrapped spatial pipeline over incoming edges", t, func() {
		nA := geometry.NewCoordinate(0, 0)
		nB := geometry.NewCoordinate(1, 0)
		weight := geometry.NewWeight(1.0, 1.0)
		edge := geometry.NewEdge(nA, nB, weight)

		mapping := geometry.NewMapping[*geometry.Coordinate](
			geometry.NewInversion(),
			geometry.NewRelaxation(),
			geometry.NewForest(),
			geometry.NewPeak(),
		)

		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(edge))
		}

		var results []*geometry.Edge
		for out := range mapping.Next(in) {
			results = append(results, (*geometry.Edge)(out))
		}

		So(mapping.Error(), ShouldBeNil)
		So(len(results), ShouldEqual, 1)
		So(results[0].Distance, ShouldAlmostEqual, 0.5)
	})
}
