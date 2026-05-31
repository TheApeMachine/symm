package broker

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
)

func TestStressMachineOutageReject(t *testing.T) {
	convey.Convey("Given an outage phase", t, func() {
		machine := &StressMachine{phase: stressOutage}
		regime := StressRegime{Turbulence: 10}

		convey.Convey("It should reject every order", func() {
			convey.So(machine.Phase(), convey.ShouldEqual, "outage")
			convey.So(machine.RejectOutcome(0.01, regime), convey.ShouldNotBeNil)
		})
	})
}

func TestStressMachineAdvance(t *testing.T) {
	convey.Convey("Given sustained turbulence", t, func() {
		machine := &StressMachine{}
		regime := StressRegime{Turbulence: 10}
		now := time.Unix(1_700_000_000, 0)

		machine.Advance(regime, now)

		convey.Convey("It should leave the healthy phase", func() {
			convey.So(machine.Phase(), convey.ShouldNotEqual, "healthy")
		})
	})
}

func TestStressMachineLatencyPenalty(t *testing.T) {
	convey.Convey("Given a degraded phase", t, func() {
		machine := &StressMachine{
			phase: stressDegraded,
		}

		latency := machine.LatencyPenalty(50*time.Millisecond, StressRegime{Turbulence: 2})

		convey.Convey("It should enforce degraded latency floor", func() {
			convey.So(latency, convey.ShouldBeGreaterThanOrEqualTo, stressDegradedLatency)
		})
	})
}

func BenchmarkStressMachineAdvance(b *testing.B) {
	machine := &StressMachine{}
	regime := StressRegime{Turbulence: 3, Vorticity: 0.5}
	now := time.Now()

	b.ReportAllocs()

	for b.Loop() {
		machine.Advance(regime, now)
	}
}
