package temporal_test

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/temporal"
)

func TestVelocity(t *testing.T) {
	Convey("Velocity computes dValue / dt from composed Delta and Divide primitives", t, func() {
		vel := temporal.NewVelocity()

		tick := func(val float64, at time.Time) float64 {
			var out float64
			for ptr := range vel.Next(data.NewValue(val, float64(at.UnixNano())).Next(nil)) {
				out = *(*float64)(ptr)
			}
			return out
		}

		t0 := time.Unix(100, 0)
		t1 := t0.Add(2 * time.Second)
		t2 := t1.Add(1 * time.Second)

		// First observation yields 0.0
		r0 := tick(10.0, t0)
		So(r0, ShouldEqual, 0.0)

		// +20 over 2 seconds -> +10/s
		r1 := tick(30.0, t1)
		So(r1, ShouldEqual, 10.0)

		// -5 over 1 second -> -5/s
		r2 := tick(25.0, t2)
		So(r2, ShouldEqual, -5.0)

		// Zero elapsed time yields 0.0 without panic
		r3 := tick(25.0, t2)
		So(r3, ShouldEqual, 0.0)
	})
}
