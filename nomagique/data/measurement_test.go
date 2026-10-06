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
	Convey("Given a finalized Measurement with a known Coherence and Maturity", t, func() {
		measurement := NewMeasurement(1, "BTC/USD", "test", 1, 1)
		measurement.At = time.Unix(1, 0).UTC()
		measurement.From = measurement.At
		measurement.coherence = 0.8
		measurement.maturity = 0.5
		measurement.ID = 1

		Convey("Confidence is temporal maturity times cross-metric coherence", func() {
			So(measurement.Confidence(), ShouldAlmostEqual, 0.5*0.8, 1e-12)
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
		unfinalized := NewMeasurement(1, "BTC/USD", "test", 1, 1)
		So(unfinalized.Confidence(), ShouldEqual, 0)
		So(errnie.IsKind(unfinalized.Error(), errnie.Forbidden), ShouldBeTrue)

		So(unfinalized.Coherence(), ShouldEqual, 0)
		So(unfinalized.Maturity(), ShouldEqual, 0)
		So(unfinalized.Peers(), ShouldBeNil)
		So(unfinalized.Meta("foo"), ShouldEqual, "")
	})
}

func TestMeasurement_Coherence(t *testing.T) {
	Convey("Given Jain's fairness index on absolute standardized magnitudes", t, func() {
		Convey("When all metrics participate equally, coherence is 1.0", func() {
			measurement := NewMeasurement(1, "BTC/USD", "test", 1, 1)
			measurement.At = time.Unix(1, 0).UTC()
			measurement.From = measurement.At

			m1 := NewMetric("m1", 1, UnitDimensionless, TimescaleInstantaneous)
			m1.Standardized = 1.0
			m2 := NewMetric("m2", 1, UnitDimensionless, TimescaleInstantaneous)
			m2.Standardized = -1.0
			m3 := NewMetric("m3", 1, UnitDimensionless, TimescaleInstantaneous)
			m3.Standardized = 1.0

			measurement.metrics = []*MetricEntry{
				{Key: "m1", Metric: m1},
				{Key: "m2", Metric: m2},
				{Key: "m3", Metric: m3},
			}

			measurement.setCoherence()
			measurement.ID = 1
			So(measurement.Coherence(), ShouldAlmostEqual, 1.0, 1e-12)
		})

		Convey("When one metric carries the entire observation, coherence is 1/N", func() {
			measurement := NewMeasurement(1, "BTC/USD", "test", 1, 1)
			measurement.At = time.Unix(1, 0).UTC()
			measurement.From = measurement.At

			m1 := NewMetric("m1", 1, UnitDimensionless, TimescaleInstantaneous)
			m1.Standardized = 3.0
			m2 := NewMetric("m2", 0, UnitDimensionless, TimescaleInstantaneous)
			m2.Standardized = 0.0
			m3 := NewMetric("m3", 0, UnitDimensionless, TimescaleInstantaneous)
			m3.Standardized = 0.0

			measurement.metrics = []*MetricEntry{
				{Key: "m1", Metric: m1},
				{Key: "m2", Metric: m2},
				{Key: "m3", Metric: m3},
			}

			measurement.setCoherence()
			measurement.ID = 1
			So(measurement.Coherence(), ShouldAlmostEqual, 1.0/3.0, 1e-12)
		})

		Convey("When metrics are moderately uneven [1, 2, 3], coherence is 36/42", func() {
			measurement := NewMeasurement(1, "BTC/USD", "test", 1, 1)
			measurement.At = time.Unix(1, 0).UTC()
			measurement.From = measurement.At

			m1 := NewMetric("m1", 1, UnitDimensionless, TimescaleInstantaneous)
			m1.Standardized = 1.0
			m2 := NewMetric("m2", 2, UnitDimensionless, TimescaleInstantaneous)
			m2.Standardized = 2.0
			m3 := NewMetric("m3", 3, UnitDimensionless, TimescaleInstantaneous)
			m3.Standardized = 3.0

			measurement.metrics = []*MetricEntry{
				{Key: "m1", Metric: m1},
				{Key: "m2", Metric: m2},
				{Key: "m3", Metric: m3},
			}

			measurement.setCoherence()
			measurement.ID = 1
			So(measurement.Coherence(), ShouldAlmostEqual, 36.0/42.0, 1e-12)
		})

		Convey("When all metrics are zero, coherence is 0", func() {
			measurement := NewMeasurement(1, "BTC/USD", "test", 1, 1)
			measurement.At = time.Unix(1, 0).UTC()
			measurement.From = measurement.At

			m1 := NewMetric("m1", 0, UnitDimensionless, TimescaleInstantaneous)
			m1.Standardized = 0.0
			m2 := NewMetric("m2", 0, UnitDimensionless, TimescaleInstantaneous)
			m2.Standardized = 0.0

			measurement.metrics = []*MetricEntry{
				{Key: "m1", Metric: m1},
				{Key: "m2", Metric: m2},
			}

			measurement.setCoherence()
			measurement.ID = 1
			So(measurement.Coherence(), ShouldEqual, 0)
		})
	})
}
