package geometry_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/geometry"
)

func TestInversionNext(t *testing.T) {
	Convey("Inversion transforms sympathy into metric distances", t, func() {
		inversion := geometry.NewInversion()
		strengths := []float64{1.0, -0.5}
		in := func(yield func(unsafe.Pointer) bool) {
			for idx := range strengths {
				if !yield(unsafe.Pointer(&strengths[idx])) {
					return
				}
			}
		}

		var distances []float64
		for out := range inversion.Next(in) {
			distances = append(distances, *(*float64)(out))
		}

		So(inversion.Error(), ShouldBeNil)
		So(len(distances), ShouldEqual, 2)
		So(distances[0], ShouldAlmostEqual, 0.5)
		So(distances[1], ShouldAlmostEqual, 1.5)
	})
}
