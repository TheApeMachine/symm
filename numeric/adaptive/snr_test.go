package adaptive

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSNRScore(t *testing.T) {
	Convey("Given a fresh SNR tracker", t, func() {
		snr := NewSNR()

		Convey("It should report 0 while warming up", func() {
			score := 0.0

			for range defaultSNRMinObs - 1 {
				score = snr.Score(1.0)
			}

			So(score, ShouldEqual, 0)
		})

		Convey("It should stay near 0 for a steady stream then spike on an outlier", func() {
			// Seed a noisy-but-stationary baseline around 1.0.
			for index := range 200 {
				if index%2 == 0 {
					snr.Score(1.1)
				} else {
					snr.Score(0.9)
				}
			}

			steady := snr.Score(1.0)
			spike := snr.Score(5.0)

			Convey("the steady reading is below the noise floor", func() {
				So(steady, ShouldBeLessThan, 1)
			})

			Convey("the outlier clears it by several sigma", func() {
				So(spike, ShouldBeGreaterThan, 3)
			})
		})

		Convey("It should never return a negative SNR", func() {
			for range 50 {
				snr.Score(10.0)
			}

			So(snr.Score(0.0), ShouldEqual, 0)
		})
	})
}

func TestSNRFieldScore(t *testing.T) {
	Convey("Given a per-symbol SNR field", t, func() {
		field := NewSNRField()

		// Two symbols on completely different scales, each with its own noise.
		for index := range 200 {
			high, low := 101.0, 99.0
			dHigh, dLow := 0.011, 0.009

			if index%2 == 1 {
				high, low = low, high
				dHigh, dLow = dLow, dHigh
			}

			field.Score("BTC/EUR", high)
			field.Score("BTC/EUR", low)
			field.Score("DOGE/EUR", dHigh)
			field.Score("DOGE/EUR", dLow)
		}

		Convey("It should normalize each symbol against its own noise", func() {
			// A within-band reading is quiet; an outlier (relative to that
			// symbol's own scale) clears the floor — for both, despite the
			// 10,000x difference in raw magnitude.
			So(field.Score("BTC/EUR", 100.0), ShouldBeLessThan, 1)
			So(field.Score("BTC/EUR", 140.0), ShouldBeGreaterThan, 3)
			So(field.Score("DOGE/EUR", 0.010), ShouldBeLessThan, 1)
			So(field.Score("DOGE/EUR", 0.014), ShouldBeGreaterThan, 3)
		})
	})
}
