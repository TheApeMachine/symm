package learning

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPaceController(t *testing.T) {
	Convey("Given a stationary reconstruction error stream", t, func() {
		controller := NewPaceController()
		alpha := defaultRestAlpha
		early := 0.0

		for tick := range 20_000 {
			alpha = controller.Update(1.0 + 0.01*math.Sin(float64(tick)))

			if tick == 2_000 {
				early = alpha
			}
		}

		Convey("Then the pace oscillates about rest rather than walking to a bound", func() {
			So(alpha, ShouldBeBetween, defaultRestAlpha/2, defaultRestAlpha*2)
			So(alpha, ShouldBeLessThan, defaultMaxAlpha)
			So(alpha, ShouldBeGreaterThan, defaultMinAlpha)
		})

		Convey("Then ten times more ticks does not compound the excursion", func() {
			So(math.Abs(alpha-defaultRestAlpha), ShouldBeLessThan, defaultRestAlpha)
			So(math.Abs(early-defaultRestAlpha), ShouldBeLessThan, defaultRestAlpha)
		})
	})

	Convey("Given an error stream with intermittent spikes", t, func() {
		controller := NewPaceController()
		alpha := defaultRestAlpha

		for tick := range 20_000 {
			reading := 1.0 + 0.01*math.Sin(float64(tick))

			if tick%7 == 0 {
				reading = 5.0
			}

			alpha = controller.Update(reading)
		}

		Convey("Then the pace stays near rest instead of pinning at the ceiling", func() {
			So(alpha, ShouldBeLessThan, defaultMaxAlpha)
			So(alpha, ShouldBeGreaterThan, defaultMinAlpha)
			So(alpha, ShouldBeLessThan, defaultRestAlpha*2)
		})
	})

	Convey("Given a quiet stretch followed by a sustained error rise", t, func() {
		controller := NewPaceController()

		for tick := range 300 {
			controller.Update(1.0 + 0.01*math.Sin(float64(tick)))
		}

		quiet := controller.Update(1.0)

		for range 300 {
			controller.Update(50.0)
		}

		shifted := controller.Update(50.0)

		Convey("Then the pace rises off the quiet level and stays bounded", func() {
			So(quiet, ShouldBeBetween, defaultRestAlpha/2, defaultRestAlpha*2)
			So(shifted, ShouldBeGreaterThan, quiet*2)
			So(shifted, ShouldBeLessThanOrEqualTo, defaultMaxAlpha)
		})
	})

	Convey("Given a controller during initial warmup", t, func() {
		controller := NewPaceController()
		alpha := controller.Update(7.0)

		Convey("Then the pace is held at initial resting rate", func() {
			So(alpha, ShouldEqual, defaultRestAlpha)
		})
	})

	Convey("Given non-finite inputs to pace controller", t, func() {
		controller := NewPaceController()
		_, err := controller.Measure(math.NaN())

		Convey("Then a validation error is returned", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkPaceController(b *testing.B) {
	controller := NewPaceController()

	for b.Loop() {
		_ = controller.Update(1.234)
	}
}
