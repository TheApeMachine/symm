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
				var err error
				score, err = snr.Score(0.5)

				So(err, ShouldBeNil)
			}

			So(score, ShouldEqual, 0)
		})

		Convey("It should stay near 0 for a steady stream then spike on an outlier", func() {
			for index := range 200 {
				if index%2 == 0 {
					_, err := snr.Score(0.55)
					So(err, ShouldBeNil)
				} else {
					_, err := snr.Score(0.45)
					So(err, ShouldBeNil)
				}
			}

			steady, err := snr.Score(0.5)
			So(err, ShouldBeNil)

			spike, err := snr.Score(0.95)
			So(err, ShouldBeNil)

			Convey("the steady reading is below the noise floor", func() {
				So(steady, ShouldBeLessThan, 1)
			})

			Convey("the outlier clears it by several sigma", func() {
				So(spike, ShouldBeGreaterThan, 3)
			})
		})

		Convey("It should never return a negative SNR", func() {
			for index := range 50 {
				value := 0.55

				if index%2 == 1 {
					value = 0.45
				}

				_, err := snr.Score(value)
				So(err, ShouldBeNil)
			}

			score, err := snr.Score(0.3)
			So(err, ShouldBeNil)
			So(score, ShouldEqual, 0)
		})

		Convey("It should error on non-unit standout instead of silently scoring 0", func() {
			for range defaultSNRMinObs {
				_, err := snr.Score(0.5)
				So(err, ShouldBeNil)
			}

			_, err := snr.Score(1_218_322_141.582215)
			So(err, ShouldNotBeNil)
		})

		Convey("It should regularize collapsed variance instead of exploding", func() {
			for range defaultSNRMinObs + 20 {
				_, err := snr.Score(0.5)
				So(err, ShouldBeNil)
			}

			score, err := snr.Score(0.5004)
			So(err, ShouldBeNil)
			So(score, ShouldAlmostEqual, 0.02, 1e-9)
		})

		Convey("It should score a micro-jump above a flat baseline without error", func() {
			for range defaultSNRMinObs {
				_, err := snr.Score(0.2)
				So(err, ShouldBeNil)
			}

			score, err := snr.Score(0.21)
			So(err, ShouldBeNil)
			So(score, ShouldAlmostEqual, 0.5, 1e-9)
		})
	})
}

func TestSNRFieldScore(t *testing.T) {
	Convey("Given a per-symbol SNR field on unit standout", t, func() {
		field := NewSNRField()

		for index := range 200 {
			high, low := 0.55, 0.45

			if index%2 == 1 {
				high, low = low, high
			}

			_, err := field.Score("BTC/EUR", high)
			So(err, ShouldBeNil)

			_, err = field.Score("BTC/EUR", low)
			So(err, ShouldBeNil)

			_, err = field.Score("DOGE/EUR", 0.11)
			So(err, ShouldBeNil)

			_, err = field.Score("DOGE/EUR", 0.09)
			So(err, ShouldBeNil)
		}

		Convey("It should normalize each symbol against its own noise", func() {
			steady, err := field.Score("BTC/EUR", 0.5)
			So(err, ShouldBeNil)
			So(steady, ShouldBeLessThan, 1)

			spike, err := field.Score("BTC/EUR", 0.95)
			So(err, ShouldBeNil)
			So(spike, ShouldBeGreaterThan, 3)

			dogeQuiet, err := field.Score("DOGE/EUR", 0.10)
			So(err, ShouldBeNil)
			So(dogeQuiet, ShouldBeLessThan, 1)

			dogeSpike, err := field.Score("DOGE/EUR", 0.14)
			So(err, ShouldBeNil)
			So(dogeSpike, ShouldBeGreaterThan, 1)
			So(dogeSpike, ShouldBeLessThan, 10)
		})
	})
}

func BenchmarkSNRScore(b *testing.B) {
	snr := NewSNR()

	for index := range defaultSNRMinObs {
		_, _ = snr.Score(float64(index%3) * 0.1)
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = snr.Score(0.75)
	}
}
