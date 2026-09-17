package geometry_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/geometry"
)

func TestWeight(t *testing.T) {
	Convey("Weight yields strength and direction when stepped without input", t, func() {
		weight := geometry.NewWeight(0.85, 1.0)
		var values []float64

		for ptr := range weight.Next(nil) {
			values = append(values, *(*float64)(ptr))
		}

		So(len(values), ShouldEqual, 2)
		So(values[0], ShouldEqual, 0.85)
		So(values[1], ShouldEqual, 1.0)
	})

	Convey("Weight scales arriving scalars by strength", t, func() {
		weight := geometry.NewWeight(0.5, -1.0)
		inputVal := 10.0
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&inputVal))
		}

		var scaled []float64
		for ptr := range weight.Next(in) {
			scaled = append(scaled, *(*float64)(ptr))
		}

		So(len(scaled), ShouldEqual, 1)
		So(scaled[0], ShouldEqual, 5.0)
	})
}
