package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewMetric(t *testing.T) {
	Convey("Given a raw count", t, func() {
		metric := NewMetric(NewValue(20.0), Descriptor{
			Unit:      UnitCount,
			Timescale: TimescalePerSecond,
		})

		Convey("It should keep raw value and descriptor", func() {
			So(metric.Value.Read(), ShouldEqual, 20)
			So(metric.Error(), ShouldBeNil)
			So(metric.Unit(), ShouldEqual, UnitCount)
			So(metric.Timescale(), ShouldEqual, TimescalePerSecond)
		})
	})
}
