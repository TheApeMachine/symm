package market

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestGaugeReadingsMeanClarity(t *testing.T) {
	convey.Convey("Given per-symbol clarity readings", t, func() {
		readings := newGaugeReadings()

		readings.record("hawkes", "BTC/EUR", 1.0, 0)
		readings.record("hawkes", "ETH/EUR", 0.2, 0)

		mean, count := readings.meanClarity("hawkes")

		convey.Convey("It should mean across symbols instead of keeping the max", func() {
			convey.So(count, convey.ShouldEqual, 2)
			convey.So(mean, convey.ShouldAlmostEqual, 0.6, 1e-9)
		})

		readings.record("hawkes", "BTC/EUR", 0.1, 0)

		mean, count = readings.meanClarity("hawkes")

		convey.Convey("It should move down when a symbol's clarity drops", func() {
			convey.So(count, convey.ShouldEqual, 2)
			convey.So(mean, convey.ShouldAlmostEqual, 0.15, 1e-9)
		})
	})
}

func TestGaugeReadingsMeanSNR(t *testing.T) {
	convey.Convey("Given per-symbol SNR readings", t, func() {
		readings := newGaugeReadings()

		readings.record("causal", "BTC/EUR", 0.5, 2.0)
		readings.record("causal", "ETH/EUR", 0.5, 4.0)

		convey.Convey("It should mean SNR across symbols", func() {
			convey.So(readings.meanSNR("causal"), convey.ShouldAlmostEqual, 3.0, 1e-9)
		})
	})
}
