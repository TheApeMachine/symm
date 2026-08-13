package resonance

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
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
	Convey("Given accurate forecasts at three distinct horizons", t, func() {
		ledger := newHorizonLedger()

		for sample := range 8 {
			actual := 0.01 + float64(sample)*0.001

			for horizon := 1; horizon <= 3; horizon++ {
				ledger.observe(horizon, actual*0.9, actual)
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
			ledger.observe(1, actual, actual)
			ledger.observe(3, actual, actual)
		}

		Convey("It should not leap over unevaluated ground truth", func() {
			So(ledger.supported(0.95), ShouldEqual, 1)
		})
	})

	Convey("Given many tiny wins hiding catastrophic misses", t, func() {
		ledger := newHorizonLedger()

		for range 64 {
			ledger.observe(1, 0.0001, 0.0001)
		}

		for range 8 {
			ledger.observe(1, 0.1, -0.1)
		}

		Convey("It should use error magnitude rather than a win count", func() {
			So(ledger.supported(0.95), ShouldEqual, 0)
		})
	})
}

func TestHorizonLedgerSupported(t *testing.T) {
	Convey("Given a frontier whose second horizon loses skill", t, func() {
		ledger := newHorizonLedger()

		for sample := range 16 {
			actual := 0.01 + float64(sample)*0.0001
			ledger.observe(1, actual, actual)
			ledger.observe(2, -actual, actual)
		}

		Convey("It should contract to the last contiguous skilled horizon", func() {
			So(ledger.supported(0.95), ShouldEqual, 1)
		})
	})

	Convey("Given only one resolved forecast", t, func() {
		ledger := newHorizonLedger()
		ledger.observe(1, 0.01, 0.01)

		Convey("It should not invent variance from one outcome", func() {
			So(ledger.supported(0.5), ShouldEqual, 0)
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
