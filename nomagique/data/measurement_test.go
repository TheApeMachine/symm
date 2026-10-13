package data

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMeasurementNext(t *testing.T) {
	Convey("Given a Measurement and consecutive transitions via Next", t, func() {
		epoch := int64(1)
		source := "spot:trade"
		label := "BTC/USD"

		first := NewMeasurement(epoch, label, source, 1, 1)
		first.At = time.Now().UTC()
		first.From = first.At

		first.copyMetrics(source, map[string]float64{
			"price": 100.0,
			"qty":   2.0,
		}, nil)
		first.finalize()

		Convey("First observation has zero SNM and zero energy", func() {
			for entry := range first.Read("price") {
				So(entry.Metric.SNM(), ShouldEqual, 0.0)
			}
			So(first.Energy(), ShouldEqual, 0.0)
		})

		Convey("Next transition calculates SNM against prior observation", func() {
			second := first.Next(map[string]float64{
				"price": 110.0,
				"qty":   2.0,
			})

			for entry := range second.Read("price") {
				expected := 10.0 / 110.0
				So(entry.Metric.SNM(), ShouldAlmostEqual, expected, 0.0001)
			}

			// qty did not move (2.0 -> 2.0), so qty SNM is 0
			for entry := range second.Read("qty") {
				So(entry.Metric.SNM(), ShouldEqual, 0.0)
			}

			// Energy is sum of squared SNMs
			priceSNM := 10.0 / 110.0
			expectedEnergy := priceSNM * priceSNM
			So(second.Energy(), ShouldAlmostEqual, expectedEnergy, 0.0001)
		})
	})
}

func TestMeasurementWrite(t *testing.T) {
	Convey("Given a Measurement and metrics passed to Write", t, func() {
		epoch := int64(1)
		source := "spot:trade"
		label := "ETH/USD"

		Convey("When metrics have pre-calculated SNM, Write preserves it", func() {
			measurement := NewMeasurement(epoch, label, source, 1, 1)
			measurement.At = time.Now().UTC()
			measurement.From = measurement.At

			priceMetric := NewMetric("price", 3050.0, UnitPrice, TimescaleInstantaneous)
			priceMetric.setSNM(3000.0)
			qtyMetric := NewMetric("qty", 5.0, UnitQuantity, TimescaleInstantaneous)

			measurement.Write(priceMetric, qtyMetric)

			for entry := range measurement.Read("price") {
				So(entry.Metric.SNM(), ShouldBeGreaterThan, 0.0)
			}

			So(measurement.Energy(), ShouldBeGreaterThan, 0.0)
		})

		Convey("When updating existing metrics in Write, SetSNM is computed from prior raw", func() {
			measurement := NewMeasurement(epoch, label, source, 1, 1)
			measurement.At = time.Now().UTC()
			measurement.From = measurement.At

			// Initial metric without locking
			measurement.metrics = append(measurement.metrics, &MetricEntry{
				Key:    "price",
				Metric: NewMetric("price", 3000.0, UnitPrice, TimescaleInstantaneous),
			})

			// Write updated price
			newPrice := NewMetric("price", 3100.0, UnitPrice, TimescaleInstantaneous)
			measurement.Write(newPrice)

			for entry := range measurement.Read("price") {
				expected := 100.0 / (3000.0 + 100.0)
				So(entry.Metric.SNM(), ShouldAlmostEqual, expected, 0.0001)
			}
		})
	})
}
