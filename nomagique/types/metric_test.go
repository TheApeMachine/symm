package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewMetric(t *testing.T) {
	Convey("Given a raw count", t, func() {
		metric := NewMetric(20.0, UnitCount, TimescalePerSecond)
		var input Input[float64] = metric

		Convey("It should wire as input and keep normalization absent", func() {
			So(input.Value(), ShouldEqual, 20)
			So(input.Error(), ShouldBeNil)
			So(metric.Unit(), ShouldEqual, UnitCount)
			So(metric.Timescale(), ShouldEqual, TimescalePerSecond)
			So(metric.Normalized(), ShouldBeNil)
		})

		Convey("When normalized", func() {
			metric.Normalize(0.4)

			Convey("It should keep the raw value as the wire payload", func() {
				So(metric.Value(), ShouldEqual, 20)
				So(*metric.Normalized(), ShouldEqual, 0.4)
			})
		})
	})
}
