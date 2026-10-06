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

func TestMeasurement_Confidence(t *testing.T) {
	Convey("Given a finalized Measurement with a known SNR and Maturity", t, func() {
		measurement := NewMeasurement(1, "BTC/USD", "test", 1, 1).Restore(2, 0.5)
		measurement.At = time.Unix(1, 0).UTC()
		measurement.From = measurement.At
		measurement.Write(NewMetric("price", 42.5, UnitCurrency, TimescaleInstantaneous))

		Convey("Confidence is Maturity times the Wiener gain SNR^2 / (1 + SNR^2)", func() {
			So(measurement.Confidence(), ShouldAlmostEqual, 0.5*4.0/5.0, 1e-12)
		})
	})

	Convey("Given a Measurement that observed one sample of its regime", t, func() {
		measurement := NewMeasurement(1, "BTC/USD", "test", 1, 1)
		measurement.At = time.Unix(1, 0).UTC()
		measurement.From = measurement.At
		measurement.Write(NewMetric("price", 42.5, UnitCurrency, TimescaleInstantaneous))

		Convey("It is immature, so none of its Metrics is trusted", func() {
			So(measurement.Confidence(), ShouldEqual, 0)
		})
	})

	Convey("Given a Measurement that is not finalized", t, func() {
		So(NewMeasurement(1, "BTC/USD", "test", 1, 1).Confidence(), ShouldEqual, 0)
	})
}

func TestMeasurement_Restore(t *testing.T) {
	Convey("Given a replayed Measurement restored with its stored SNR and Maturity", t, func() {
		measurement := NewMeasurement(1, "BTC/USD", "test", 1, 1).Restore(1.25, 0.75)
		measurement.At = time.Unix(1, 0).UTC()
		measurement.From = measurement.At
		measurement.Write(NewMetric("price", 42.5, UnitCurrency, TimescaleInstantaneous))

		Convey("Write keeps them instead of re-deriving them from one sample", func() {
			So(measurement.SNR(), ShouldEqual, 1.25)
			So(measurement.Maturity(), ShouldEqual, 0.75)
		})

		Convey("A finalized Measurement cannot be restored again", func() {
			measurement.Restore(9, 0.1)

			So(errnie.IsKind(measurement.Error(), errnie.Forbidden), ShouldBeTrue)
			So(measurement.SNR(), ShouldEqual, 1.25)
			So(measurement.Maturity(), ShouldEqual, 0.75)
		})
	})
}
