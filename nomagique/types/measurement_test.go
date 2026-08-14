package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewMeasurement(t *testing.T) {
	Convey("Given a measurement with one named metric", t, func() {
		measurement := NewMeasurement("1", "hawkes")
		measurement.Put("event_count", NewMetric(7.0, UnitCount, TimescalePerSecond))
		var input Input[*Measurement] = measurement

		Convey("It should expose the named metric as the next stage's input", func() {
			So(input.Value(), ShouldEqual, measurement)
			So(input.Error(), ShouldBeNil)
			So(measurement.Metric("event_count").Value(), ShouldEqual, 7)
			So(measurement.Metric("event_count").Error(), ShouldBeNil)
		})

		Convey("It should fail a missing metric without inventing a reading", func() {
			missing := measurement.Metric("decay")
			So(missing.Value(), ShouldEqual, 0)
			So(missing.Error(), ShouldNotBeNil)
		})
	})
}
