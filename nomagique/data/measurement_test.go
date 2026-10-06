package data

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
)

func TestMeasurementFinalize(t *testing.T) {
	Convey("Given a fresh Measurement", t, func() {
		measurement := NewMeasurement(1, "BTC/USD", "test", 1, 1)
		measurement.At = time.Unix(1, 0).UTC()
		measurement.From = measurement.At

		Convey("Read before Write is Forbidden without a metric", func() {
			var entries []*MetricEntry

			for entry := range measurement.Read("price") {
				entries = append(entries, entry)
			}

			So(entries, ShouldHaveLength, 1)
			So(entries[0].Metric, ShouldBeNil)
			So(errnie.IsKind(entries[0].Err, errnie.Forbidden), ShouldBeTrue)
		})

		Convey("Write counts the observation before metric Welford updates", func() {
			metric := NewMetric("price", 42.5, UnitCurrency, TimescaleInstantaneous)
			measurement.Write(metric)

			So(measurement.locked(), ShouldBeTrue)
			So(measurement.samples, ShouldEqual, 1)
			So(math.IsNaN(metric.center) || math.IsInf(metric.center, 0), ShouldBeFalse)
			So(metric.center, ShouldEqual, 42.5)
			So(math.IsNaN(measurement.Maturity()), ShouldBeFalse)

			var read []*MetricEntry

			for entry := range measurement.Read("price") {
				read = append(read, entry)
			}

			So(read, ShouldHaveLength, 1)
			So(read[0].Err, ShouldBeNil)
			So(read[0].Metric.Raw, ShouldEqual, 42.5)
		})

		Convey("A second Write is rejected by the WORM lock", func() {
			measurement.Write(NewMetric("price", 1, UnitCurrency, TimescaleInstantaneous))
			measurement.Write(NewMetric("price", 2, UnitCurrency, TimescaleInstantaneous))

			So(errnie.IsKind(measurement.Error(), errnie.Forbidden), ShouldBeTrue)
			So(measurement.samples, ShouldEqual, 1)
		})
	})
}
