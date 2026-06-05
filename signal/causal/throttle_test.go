package causal

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestCausalVelocityThrottle(t *testing.T) {
	Convey("Given a causal fit throttle", t, func() {
		base := time.Unix(1_700_000_000, 0)

		Convey("A calm market refits only on the time gate", func() {
			signal := &Signal{}

			So(signal.throttle(base, 0.30), ShouldBeTrue)                            // first fit
			So(signal.throttle(base.Add(100*time.Millisecond), 0.31), ShouldBeFalse) // tiny shift, within interval
			So(signal.throttle(base.Add(causalPublishInterval), 0.31), ShouldBeTrue) // interval elapsed
		})

		Convey("A contagion spike bypasses the timer immediately", func() {
			signal := &Signal{}

			So(signal.throttle(base, 0.30), ShouldBeTrue) // fit at 0.30
			// Well within the 500ms interval, but contagion jumped 0.30 -> 0.60.
			So(signal.throttle(base.Add(50*time.Millisecond), 0.60), ShouldBeTrue)
		})

		Convey("The exponential backoff caps a violent ramp", func() {
			signal := &Signal{}

			So(signal.throttle(base, 0.30), ShouldBeTrue)                          // fit at 0.30
			So(signal.throttle(base.Add(50*time.Millisecond), 0.60), ShouldBeTrue) // emergency refit, backoff 100ms

			// Another spike 20ms later — still inside the 100ms backoff — is suppressed.
			So(signal.throttle(base.Add(70*time.Millisecond), 0.90), ShouldBeFalse)

			// Once the backoff expires (still within the time interval) it can refit again,
			// and the backoff then escalates to 200ms.
			So(signal.throttle(base.Add(200*time.Millisecond), 0.90), ShouldBeTrue)
			So(signal.throttle(base.Add(300*time.Millisecond), 1.00), ShouldBeFalse) // inside the 200ms backoff
		})

		Convey("A normal time-gated refit resets the backoff", func() {
			signal := &Signal{}

			So(signal.throttle(base, 0.30), ShouldBeTrue)
			So(signal.throttle(base.Add(50*time.Millisecond), 0.60), ShouldBeTrue) // emergency -> backoff armed
			So(signal.emergencyBackoff, ShouldBeGreaterThan, 0)

			So(signal.throttle(base.Add(600*time.Millisecond), 0.60), ShouldBeTrue) // time gate
			So(signal.emergencyBackoff, ShouldEqual, 0)                             // storm over, reset
		})
	})
}

func TestCausalPublishIntervalForRegime(t *testing.T) {
	Convey("Given the causal resource-aware scheduler", t, func() {
		Convey("Calm regimes refit faster than hostile ones", func() {
			So(
				causalPublishIntervalForRegime(perspectives.RegimeTrending),
				ShouldBeLessThan,
				causalPublishIntervalForRegime(perspectives.RegimeBearish),
			)
		})
	})
}
