package temporal_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/temporal"
)

func TestTemporalAtoms(t *testing.T) {
	Convey("Given temporal atoms", t, func() {
		Convey("Transition produces previous->current transitions", func() {
			trans := temporal.NewTransition()

			first := trans([]byte("r0"))
			So(first, ShouldBeNil)

			second := trans([]byte("r1"))
			So(string(second), ShouldEqual, "r0->r1")

			third := trans([]byte("r2"))
			So(string(third), ShouldEqual, "r1->r2")
		})

		Convey("Velocity computes finite difference rate", func() {
			vel := temporal.NewVelocity()
			v0 := vel([2]float64{100.0, 1.0})
			So(v0, ShouldEqual, 0.0)

			v1 := vel([2]float64{110.0, 3.0}) // (110-100)/(3-1) = 5.0
			So(v1, ShouldEqual, 5.0)
		})
	})
}
