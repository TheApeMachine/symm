package leadlag

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRecentPathMove(t *testing.T) {
	Convey("Given a flat anchor path across the lag window", t, func() {
		state := newSymbolState()
		start := time.Now()

		for index := range minLagSamples {
			state.observeTicker(50000, start.Add(time.Duration(index)*2*time.Minute))
		}

		move, ok := state.recentPathMove(anchorMoveWindow())

		Convey("It should report a near-zero move", func() {
			So(ok, ShouldBeTrue)
			So(move, ShouldBeLessThan, 1e-6)
		})
	})

	Convey("Given a trending anchor path across the lag window", t, func() {
		state := newSymbolState()
		start := time.Now()

		for index := range minLagSamples {
			state.observeTicker(50000+float64(index)*50, start.Add(time.Duration(index)*2*time.Minute))
		}

		move, ok := state.recentPathMove(anchorMoveWindow())

		Convey("It should report a positive move", func() {
			So(ok, ShouldBeTrue)
			So(move, ShouldBeGreaterThan, 0)
		})
	})
}

func TestMoveBaselineEvaluate(t *testing.T) {
	Convey("Given a warmed move baseline", t, func() {
		baseline := newMoveBaseline()

		for index := range anchorMoveMinObs {
			_, _, ready := baseline.evaluate(0.0001 + float64(index%2)*0.00005)
			So(ready, ShouldBeFalse)
		}

		Convey("It should classify a flat reading as stall with unit margin", func() {
			moved, margin, ready := baseline.evaluate(0.00001)
			So(ready, ShouldBeTrue)
			So(moved, ShouldBeFalse)
			So(margin, ShouldBeGreaterThan, 0)
			So(margin, ShouldBeLessThanOrEqualTo, 1)
		})

		Convey("It should classify a large spike as moved", func() {
			moved, _, ready := baseline.evaluate(0.05)
			So(ready, ShouldBeTrue)
			So(moved, ShouldBeTrue)
		})
	})
}
