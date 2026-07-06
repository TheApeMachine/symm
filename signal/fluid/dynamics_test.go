package fluid

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFluidDynamicsIcebergScore(t *testing.T) {
	Convey("Given balanced add and execute rates at the touch", t, func() {
		dynamics := fluidDynamics{}
		base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		pairs := [][2]float64{{10, 10}, {12, 11}, {9, 9}, {8, 8}}

		for index, pair := range pairs {
			dynamics.record(
				base.Add(time.Duration(index)*time.Second),
				1, 1, 1, 1, 1,
				pair[0], pair[1],
			)
		}

		score := dynamics.icebergScore(10, 10)

		Convey("It should score hidden absorption when rates balance", func() {
			So(score, ShouldEqual, 10)
		})
	})
}

func TestFluidDynamicsIcebergFiresOnFirstObservation(t *testing.T) {
	Convey("Given a single recorded balance sample", t, func() {
		dynamics := fluidDynamics{}
		dynamics.record(
			time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			1, 1, 1, 1, 1,
			10, 10,
		)

		floor, ready := dynamics.icebergBalanceFloor()

		Convey("It should emit a floor on first observation, not gate on a warmup count", func() {
			So(ready, ShouldBeTrue)
			So(floor, ShouldBeGreaterThan, 0)
		})
	})
}
