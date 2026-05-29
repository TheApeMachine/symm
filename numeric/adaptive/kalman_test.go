package adaptive

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestScalarKalmanMatchesEMAInCalmConditions(t *testing.T) {
	Convey("Given a filter fed a steady stream with an ordinary step", t, func() {
		kalman := NewScalarKalman(0.3, 0.2, 2)
		kalman.Observe(1, 0.2, 3)

		for i := 0; i < 30; i++ {
			kalman.Observe(1, 0.2, 3)
		}

		Convey("An ordinary-sized step should move the state at the calm gain", func() {
			before := kalman.State()
			sigma := kalman.Sigma()
			// A step of one sigma is well within the robust threshold, so the gain is calm.
			state, zscore := kalman.Observe(before+sigma, 0.2, 3)

			So(zscore, ShouldBeLessThanOrEqualTo, 3)
			So(state-before, ShouldAlmostEqual, 0.2*sigma, sigma*0.05)
		})
	})
}

func TestScalarKalmanAbsorbsShock(t *testing.T) {
	Convey("Given a filter with an established calm scale", t, func() {
		kalman := NewScalarKalman(0.05, 0.3, 3)
		kalman.Observe(1, 0.3, 3)

		for i := 0; i < 40; i++ {
			noisy := 1.0
			if i%2 == 0 {
				noisy = 1.05
			} else {
				noisy = 0.95
			}
			kalman.Observe(noisy, 0.3, 3)
		}

		before := kalman.State()

		Convey("A single far-out measurement should barely move the state", func() {
			state, zscore := kalman.Observe(10, 0.3, 3)

			So(zscore, ShouldBeGreaterThan, 3)
			// A plain EMA would jump by 0.3*(10-1) = 2.7; the robust filter rejects most of it.
			So(math.Abs(state-before), ShouldBeLessThan, 0.3*(10-before))
			So(math.Abs(state-before), ShouldBeLessThan, 0.5)
		})
	})
}

func TestScalarKalmanTracksPersistentRegimeShift(t *testing.T) {
	Convey("Given a filter anchored near one", t, func() {
		kalman := NewScalarKalman(0.05, 0.3, 3)
		kalman.Observe(1, 0.3, 3)

		for i := 0; i < 30; i++ {
			kalman.Observe(1, 0.3, 3)
		}

		Convey("A sustained shift to a new level should eventually be tracked", func() {
			for i := 0; i < 60; i++ {
				kalman.Observe(2, 0.3, 3)
			}

			So(kalman.State(), ShouldBeGreaterThan, 1.5)
		})
	})
}
