package temporal_test

import (
	"testing"
	"time"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestVelocityNext(t *testing.T) {
	Convey("Velocity yields a finite difference for each observation", t, func() {
		op := temporal.NewVelocity()
		origin := time.Unix(1700000000, 0).UnixNano()
		obs := []temporal.Observation{
			{Value: 10, At: origin},
			{Value: 13, At: origin + int64(time.Second)},
			{Value: 16, At: origin + int64(time.Second)},
			{Value: 17, At: origin + int64(time.Second) + 1},
		}
		in := func(yield func(unsafe.Pointer) bool) {
			for i := range obs {
				if !yield(unsafe.Pointer(&obs[i])) {
					return
				}
			}
		}
		out := tests.CollectSeq[temporal.VelocityReading](op.Next(in))

		So(len(out), ShouldEqual, 4)
		So(out[0].Rate, ShouldEqual, 0)
		So(out[0].Defined, ShouldBeFalse)
		So(out[1].Rate, ShouldEqual, 3)
		So(out[1].Defined, ShouldBeTrue)
		So(out[2].Rate, ShouldEqual, 0)
		So(out[2].Defined, ShouldBeFalse)
		So(out[3].Rate, ShouldEqual, 1/(1/float64(time.Second)))
		So(out[3].Defined, ShouldBeTrue)
	})

	Convey("Given a finite difference with one previous observation via Next", t, func() {
		velocity := temporal.NewVelocity()
		fixtures := []struct {
			obs     temporal.Observation
			rate    float64
			defined bool
		}{
			{temporal.Observation{Value: 10, At: 0}, 0, false},
			{temporal.Observation{Value: 13, At: int64(time.Second)}, 3, true},
			{temporal.Observation{Value: 16, At: int64(time.Second)}, 0, false},
			{temporal.Observation{Value: 17, At: int64(time.Second) + 1}, 1 / (1 / float64(time.Second)), true},
			{temporal.Observation{Value: 11, At: int64(time.Second)}, 0, false},
			{temporal.Observation{Value: 8, At: int64(2 * time.Second)}, -3, true},
		}

		for _, fixture := range fixtures {
			in := func(yield func(unsafe.Pointer) bool) {
				yield(unsafe.Pointer(&fixture.obs))
			}
			out := tests.CollectSeq[temporal.VelocityReading](velocity.Next(in))
			So(len(out), ShouldEqual, 1)
			So(out[0].Rate, ShouldEqual, fixture.rate)
			So(out[0].Defined, ShouldEqual, fixture.defined)
		}
	})
}
