package learned

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestForecastScaleColdStart(t *testing.T) {
	Convey("Given a fresh forecast learner", t, func() {
		forecast := NewForecast(0.35)

		Convey("It should start at unit scale", func() {
			So(forecast.Scale(), ShouldEqual, 1)
		})
	})
}

func TestForecastNextLowersScaleOnLosses(t *testing.T) {
	Convey("Given one winning then one losing forecast", t, func() {
		forecast := NewForecast(0.35)

		_, err := forecast.Next(0, 0.01, 0.01)

		So(err, ShouldBeNil)

		_, err = forecast.Next(0, 0.01, -0.01)

		So(err, ShouldBeNil)
		So(forecast.Scale(), ShouldBeLessThan, 1)
	})
}

func TestForecastAbsorbUsesExplicitAlpha(t *testing.T) {
	Convey("Given repeated losses with fixed alpha", t, func() {
		forecast := NewForecast(0.35)

		for range 4 {
			So(forecast.Absorb(0.01, -0.01, 0.5), ShouldBeNil)
		}

		Convey("It should push scale below one", func() {
			So(forecast.Scale(), ShouldBeLessThan, 1)
		})
	})
}

func TestForecastReset(t *testing.T) {
	Convey("Reset clears learned scale", t, func() {
		forecast := NewForecast(0.35)
		_, _ = forecast.Next(0, 0.01, -0.01)

		So(forecast.Reset(), ShouldBeNil)
		So(forecast.Scale(), ShouldEqual, 1)
	})
}

func BenchmarkForecastNext(b *testing.B) {
	forecast := NewForecast(0.35)

	b.ResetTimer()

	for b.Loop() {
		_, _ = forecast.Next(0, 0.01, 0.008)
	}
}
