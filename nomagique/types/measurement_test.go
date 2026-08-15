package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewMeasurement(t *testing.T) {
	Convey("Given a measurement with one named metric", t, func() {
		measurement := NewMeasurement("1", "hawkes")
		measurement.Put("event_count", NewMetric(NewValue(7.0), Descriptor{
			Unit:      UnitCount,
			Timescale: TimescalePerSecond,
		}))

		Convey("It should expose the named metric as the next stage's input", func() {
			metric := measurement.Metric("event_count")
			So(metric, ShouldNotBeNil)
			So(metric.Value.Read(), ShouldEqual, 7.0)
			So(metric.Error(), ShouldBeNil)
		})

		Convey("It should fail a missing metric without inventing a reading", func() {
			missing := measurement.Metric("decay")
			So(missing, ShouldBeNil)
			So(measurement.Error(), ShouldNotBeBlank)
		})
	})
}
