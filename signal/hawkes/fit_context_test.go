package hawkes

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewFitContext(t *testing.T) {
	Convey("Given trade arrivals", t, func() {
		start := time.Now()
		stream := NewArrivalStream(
			[]time.Time{start, start.Add(time.Second), start.Add(2 * time.Second)},
			[]time.Time{start.Add(500 * time.Millisecond), start.Add(1500 * time.Millisecond)},
		)

		context, ok := NewFitContext(stream, start.Add(3*time.Second))

		Convey("It should derive fit bounds from arrivals", func() {
			So(ok, ShouldBeTrue)
			So(context.TotalEvents, ShouldEqual, 5)
			So(len(context.BetaCandidates), ShouldBeGreaterThan, 0)
		})
	})
}
