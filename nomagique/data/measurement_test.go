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

		now := time.Now().UTC()
		first := NewMeasurement(epoch, label, source, now, now)

		first.copyMetrics(source, map[string]float64{
			"price": 100.0,
			"qty":   2.0,
		}, nil, nil)
		first.finalize()

		Convey("First observation has zero SNM and zero energy", func() {
			for entry := range first.Read("price") {
				So(entry.Metric.SNM(), ShouldEqual, 0.0)
			}
			So(first.energy, ShouldEqual, 0.0)
		})

		Convey("Next transition calculates SNM against prior observation", func() {
			second := first.Next(nil, map[string]float64{
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
			So(second.energy, ShouldAlmostEqual, expectedEnergy, 0.0001)
		})
	})
}

func TestMeasurementWrite(t *testing.T) {
	Convey("Given a Measurement and metrics passed to Write", t, func() {
		epoch := int64(1)
		source := "spot:trade"
		label := "ETH/USD"

		Convey("When metrics have pre-calculated SNM, Write preserves it", func() {
			now := time.Now().UTC()
			measurement := NewMeasurement(epoch, label, source, now, now)

			priceMetric := NewMetric("price", 3050.0, UnitPrice, TimescaleInstantaneous)
			priceMetric.setSNM(3000.0)
			qtyMetric := NewMetric("qty", 5.0, UnitQuantity, TimescaleInstantaneous)

			measurement.Write(priceMetric, qtyMetric)

			for entry := range measurement.Read("price") {
				So(entry.Metric.SNM(), ShouldBeGreaterThan, 0.0)
			}

			So(measurement.energy, ShouldBeGreaterThan, 0.0)
		})

		Convey("When updating existing metrics in Write, SetSNM is computed from prior raw", func() {
			now := time.Now().UTC()
			measurement := NewMeasurement(epoch, label, source, now, now)

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

		Convey("A locked measurement rejects writes without mutating its err field", func() {
			now := time.Now().UTC()
			measurement := NewMeasurement(epoch, label, source, now, now)
			measurement.Write(NewMetric("price", 3000.0, UnitPrice, TimescaleInstantaneous))

			So(measurement.locked(), ShouldBeTrue)
			originalErr := measurement.Error()

			// Attempt to mutate locked measurement
			measurement.Write(NewMetric("qty", 10.0, UnitQuantity, TimescaleInstantaneous))
			So(measurement.Error(), ShouldEqual, originalErr)

			measurement.SetMeta("test", "forbidden")
			So(measurement.Error(), ShouldEqual, originalErr)

			peer := NewMeasurement(epoch, "BTC/USD", source, now, now)
			measurement.Peers(peer)
			So(measurement.Error(), ShouldEqual, originalErr)
		})

		Convey("Reading an unlocked measurement is strictly guarded and reports not finalized", func() {
			now := time.Now().UTC()
			unlocked := NewMeasurement(epoch, label, source, now, now)
			So(unlocked.locked(), ShouldBeFalse)

			for entry := range unlocked.Read("price") {
				So(entry.Err, ShouldNotBeNil)
			}
			So(unlocked.Error(), ShouldNotBeNil)

			for entry := range unlocked.Meta("side") {
				So(entry.Err, ShouldNotBeNil)
			}

			So(unlocked.Surprise(), ShouldEqual, 0.0)
			So(unlocked.Maturity(), ShouldEqual, 0.0)
			So(unlocked.Samples(), ShouldEqual, 0)
			So(unlocked.Energy(), ShouldEqual, 0.0)
			So(unlocked.Prediction(), ShouldEqual, 0.0)
			So(unlocked.ApproximateBytes(), ShouldEqual, 0)
			So(unlocked.Peers(), ShouldBeNil)
		})
	})
}
