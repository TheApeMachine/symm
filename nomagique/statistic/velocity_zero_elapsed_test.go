package statistic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
TestVelocity_ZeroElapsedIsNotReady pins the readiness contract. Venues batch
updates within a single clock tick, so two observations routinely share one
event timestamp. Reporting ready there published an elapsed of exactly zero,
and every consumer that divides delta by it -- each velocity chain across nine
signals -- failed its whole frame on "quotient denominator must be non-zero".
*/
func TestVelocity_ZeroElapsedIsNotReady(t *testing.T) {
	series := temporal.NewSeries("probe")
	slots := newVelocitySlots("probe")

	observe := func(frame *types.Frame, value float64, sec float64, nsec float64) {
		frame.Put(series.ValueSymbol, value)
		frame.Put(series.SecSymbol, sec)
		frame.Put(series.NsecSymbol, nsec)
		types.Step(Velocity("probe"), frame)
	}

	Convey("Given two observations sharing one event timestamp", t, func() {
		frame := types.Frame{}
		observe(&frame, 10, 1_700_000_000, 0)
		observe(&frame, 12, 1_700_000_000, 0)

		Convey("The series is not ready, so no consumer divides by zero", func() {
			ready, _ := frame.Get(series.ReadySymbol)
			So(ready, ShouldEqual, 0.0)

			elapsed, found := frame.Get(slots.elapsed)
			So(found, ShouldBeTrue)
			So(elapsed, ShouldEqual, 0.0)
		})
	})

	Convey("Given observations separated in time", t, func() {
		frame := types.Frame{}
		observe(&frame, 10, 1_700_000_000, 0)
		observe(&frame, 12, 1_700_000_001, 0)

		Convey("The series is ready and the elapsed interval is usable", func() {
			ready, _ := frame.Get(series.ReadySymbol)
			So(ready, ShouldEqual, 1.0)

			elapsed, _ := frame.Get(slots.elapsed)
			So(elapsed, ShouldAlmostEqual, 1.0, 1e-9)
		})
	})
}
