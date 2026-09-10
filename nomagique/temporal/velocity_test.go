package temporal

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestVelocityNext(t *testing.T) {
	Convey("Velocity yields a finite difference for each observation", t, func() {
		op := NewVelocity()
		origin := time.Unix(1700000000, 0).UnixNano()
		out := tests.CollectSeq(op.Next(transport.Values(
			Observation{Value: 10, At: origin},
			Observation{Value: 13, At: origin + int64(time.Second)},
			Observation{Value: 16, At: origin + int64(time.Second)},
			Observation{Value: 17, At: origin + int64(time.Second) + 1},
		)))

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
}

func TestVelocityObserve(t *testing.T) {
	Convey("Given a finite difference with one previous observation", t, func() {
		velocity := NewVelocity()
		first := velocity.Observe(10, 0)
		So(first.HasPrior, ShouldBeFalse)
		So(first.Defined, ShouldBeFalse)

		for _, fixture := range []struct {
			value   float64
			at      int64
			rate    float64
			defined bool
		}{
			{13, int64(time.Second), 3, true},
			{16, int64(time.Second), 0, false},
			{17, int64(time.Second) + 1, 1 / (1 / float64(time.Second)), true},
			{11, int64(time.Second), 0, false},
			{8, int64(2 * time.Second), -3, true},
		} {
			reading := velocity.Observe(fixture.value, fixture.at)
			So(reading.Rate, ShouldEqual, fixture.rate)
			So(reading.Defined, ShouldEqual, fixture.defined)
			So(reading.HasPrior, ShouldBeTrue)
		}

		So(first.Through.Value, ShouldEqual, 10)
		So(first.HasPrior, ShouldBeFalse)
	})
}
