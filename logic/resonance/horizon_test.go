package resonance

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/learning"
)

func TestNewHorizonLedger(t *testing.T) {
	Convey("Given a new horizon ledger", t, func() {
		ledger := newHorizonLedger()

		Convey("It should start without claiming calibrated reach", func() {
			So(ledger, ShouldNotBeNil)
			So(ledger.resolved, ShouldEqual, 0)
			So(ledger.supported(0.5), ShouldEqual, 0)
		})
	})
}

func TestHorizonLedgerObserve(t *testing.T) {
	Convey("Given matching direction calls at three distinct horizons", t, func() {
		ledger := newHorizonLedger()

		for sample := range 8 {
			actual := 0.01 + float64(sample)*0.001

			for horizon := 1; horizon <= 3; horizon++ {
				ledger.observe(horizon, 1, actual)
			}
		}

		Convey("It should support every contiguous horizon without a ceiling", func() {
			So(ledger.supported(0.95), ShouldEqual, 3)
			So(ledger.resolved, ShouldEqual, 24)
		})
	})

	Convey("Given a missing interior horizon", t, func() {
		ledger := newHorizonLedger()

		for sample := range 8 {
			actual := 0.01 + float64(sample)*0.001
			ledger.observe(1, 1, actual)
			ledger.observe(3, 1, actual)
		}

		Convey("It should not leap over unevaluated ground truth", func() {
			So(ledger.supported(0.95), ShouldEqual, 1)
		})
	})

	Convey("Given no-call forecasts and flat realizations", t, func() {
		ledger := newHorizonLedger()
		ledger.observe(1, 0, 0.01)
		ledger.observe(1, 1, 0)

		Convey("It should not treat silence as a coin-flip win or loss", func() {
			So(ledger.resolved, ShouldEqual, 0)
			So(ledger.supported(0.5), ShouldEqual, 0)
		})
	})

	Convey("Given more wrong direction calls than right ones", t, func() {
		ledger := newHorizonLedger()

		for range 8 {
			ledger.observe(1, 1, 0.01)
		}

		for range 16 {
			ledger.observe(1, 1, -0.01)
		}

		Convey("It should refuse a horizon that loses to a coin flip", func() {
			So(ledger.supported(0.95), ShouldEqual, 0)
		})
	})
}

func TestHorizonLedgerSupported(t *testing.T) {
	Convey("Given a frontier whose second horizon loses skill", t, func() {
		ledger := newHorizonLedger()

		for sample := range 16 {
			actual := 0.01 + float64(sample)*0.0001
			ledger.observe(1, 1, actual)
			ledger.observe(2, -1, actual)
		}

		Convey("It should contract to the last contiguous skilled horizon", func() {
			So(ledger.supported(0.95), ShouldEqual, 1)
		})
	})

	Convey("Given only one resolved forecast", t, func() {
		ledger := newHorizonLedger()
		ledger.observe(1, 1, 0.01)

		Convey("It should not invent variance from one outcome", func() {
			So(ledger.supported(0.5), ShouldEqual, 0)
		})
	})
}

func TestSignedDirection(t *testing.T) {
	Convey("Given signed, zero, and negative values", t, func() {
		Convey("It should report the sign without inventing a lean", func() {
			So(signedDirection(0.4), ShouldEqual, 1)
			So(signedDirection(-0.4), ShouldEqual, -1)
			So(signedDirection(0), ShouldEqual, 0)
		})
	})
}

func TestDirectionCall(t *testing.T) {
	Convey("Given a ready posterior that barely leans up", t, func() {
		output := learning.RLSOutput{
			Value: 0.01, Scale: 1, DegreesOfFreedom: 8, Ready: true,
		}

		Convey("It should call at the uninformative boundary and refuse a weak lean at a higher bar", func() {
			So(directionCall(output, 0.5), ShouldEqual, 1)
			So(directionCall(output, 0.99), ShouldEqual, 0)
		})
	})

	Convey("Given an unready or flat posterior", t, func() {
		Convey("It should not call", func() {
			So(directionCall(learning.RLSOutput{Value: 0.01, Ready: true}, 0.5),
				ShouldEqual, 0)
			So(directionCall(learning.RLSOutput{
				Value: 0, Scale: 0.1, DegreesOfFreedom: 8, Ready: true,
			}, 0.5), ShouldEqual, 0)
		})
	})
}

func BenchmarkHorizonLedgerObserve(b *testing.B) {
	ledger := newHorizonLedger()

	for b.Loop() {
		ledger.observe(16, 0.001, 0.0011)
	}
}

func BenchmarkHorizonLedgerSupported(b *testing.B) {
	ledger := newHorizonLedger()

	for horizon := 1; horizon <= 128; horizon++ {
		ledger.observe(horizon, 0.001, 0.001)
		ledger.observe(horizon, 0.0011, 0.0011)
	}

	for b.Loop() {
		_ = ledger.supported(0.95)
	}
}

func TestStabilizeDirection(t *testing.T) {
	Convey("Given an accepted upward state and a weak downward challenger", t, func() {
		stabilized := stabilizeDirection(
			1,
			directionEvidence{candidate: -1, confidence: 0.8},
			0.5,
			0.95,
		)

		Convey("It should retain continuity without publishing stale action", func() {
			So(stabilized.candidate, ShouldEqual, -1.0)
			So(stabilized.stable, ShouldEqual, 1.0)
			So(stabilized.call, ShouldEqual, 0.0)
			So(stabilized.held, ShouldBeTrue)
		})
	})

	Convey("Given an opposing posterior that clears the switch confidence", t, func() {
		stabilized := stabilizeDirection(
			1,
			directionEvidence{candidate: -1, confidence: 0.99},
			0.5,
			0.95,
		)

		Convey("It should accept the new direction immediately", func() {
			So(stabilized.stable, ShouldEqual, -1.0)
			So(stabilized.call, ShouldEqual, -1.0)
			So(stabilized.held, ShouldBeFalse)
		})
	})
}
