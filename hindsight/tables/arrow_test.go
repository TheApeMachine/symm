package tables

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestArrow_FillAndReadMeasurements(t *testing.T) {
	Convey("Given a set of measurements", t, func() {
		schema := MeasurementSchema()
		converted, err := arrowSchemaFor(schema)
		So(err, ShouldBeNil)

		price, err := decimal.NewFromString("65432.10")
		So(err, ShouldBeNil)

		at := time.Date(2026, 10, 6, 2, 0, 0, 0, time.UTC)

		measurement := data.NewMeasurement(
			100,
			"BTC/USD",
			"kraken",
			1,
			42,
			&data.StringEntry{Key: "side", Value: "bid"},
			&data.StringEntry{Key: "type", Value: "limit"},
		)
		measurement.At = at
		measurement.Restore(1.25, 0.75)
		measurement.Write(
			data.NewExactMetric("price", price, data.UnitCurrency, data.TimescaleInstantaneous),
			data.NewMetric("volume", 1.5, data.UnitVolume, data.TimescaleInstantaneous),
		)

		measurements := []*data.Measurement{measurement}

		Convey("When fillMeasurements is called", func() {
			recordBuilder := array.NewRecordBuilder(memory.DefaultAllocator, converted)
			defer recordBuilder.Release()

			So(fillMeasurements(recordBuilder, measurements, 100), ShouldBeNil)
			batch := recordBuilder.NewRecordBatch()
			defer batch.Release()

			So(batch.NumRows(), ShouldEqual, 1)
			So(batch.NumCols(), ShouldEqual, 12)

			Convey("Then ReadMeasurements reconstructs the measurement faithfully", func() {
				read, err := ReadMeasurements(batch)
				So(err, ShouldBeNil)
				So(len(read), ShouldEqual, 1)

				reconstructed := read[0]
				So(reconstructed.Epoch, ShouldEqual, 100)
				So(reconstructed.Label, ShouldEqual, "BTC/USD")
				So(reconstructed.Source, ShouldEqual, "kraken")
				So(reconstructed.SeqIdx, ShouldEqual, 1)
				So(reconstructed.Tick, ShouldEqual, 42)
				So(reconstructed.At.Equal(at), ShouldBeTrue)
				So(reconstructed.Meta("side"), ShouldEqual, "bid")
				So(reconstructed.Meta("type"), ShouldEqual, "limit")
				So(reconstructed.SNR(), ShouldEqual, 1.25)
				So(reconstructed.Maturity(), ShouldEqual, 0.75)

				priceMetric := data.Pull(reconstructed.Read("price"))
				So(priceMetric.Err, ShouldBeNil)
				So(priceMetric.Metric, ShouldNotBeNil)
				So(priceMetric.Metric.Exact, ShouldNotBeNil)
				So(priceMetric.Metric.Exact.String(), ShouldEqual, "65432.10")
				So(priceMetric.Metric.Raw, ShouldEqual, 65432.10)

				volumeMetric := data.Pull(reconstructed.Read("volume"))
				So(volumeMetric.Err, ShouldBeNil)
				So(volumeMetric.Metric, ShouldNotBeNil)
				So(volumeMetric.Metric.Raw, ShouldEqual, 1.5)
			})
		})

		Convey("When a stored row carries a metric with an undefined raw", func() {
			// The tape a pre-fix producer stored: a zscore divided 0/0.
			corrupt := data.NewMeasurement(100, "BTC/USD", "depthflow", 3, 44)
			corrupt.At = at
			corrupt.Restore(1.25, 0.75)
			corrupt.Write(
				data.NewMetric("book_imbalance", 0.1, data.UnitRatio, data.TimescaleInstantaneous),
				data.NewMetric("turnover_zscore", math.NaN(), data.UnitZScore, data.TimescaleRollingWindow),
			)

			recordBuilder := array.NewRecordBuilder(memory.DefaultAllocator, converted)
			defer recordBuilder.Release()

			So(fillMeasurements(recordBuilder, []*data.Measurement{measurement, corrupt}, 100), ShouldBeNil)
			batch := recordBuilder.NewRecordBatch()
			defer batch.Release()

			Convey("Then ReadMeasurements halts on that row instead of replaying it", func() {
				read, err := ReadMeasurements(batch)
				So(read, ShouldBeNil)
				So(err, ShouldNotBeNil)
				So(strings.Contains(err.Error(), "row 1 is invalid"), ShouldBeTrue)
				So(strings.Contains(err.Error(), "raw is required"), ShouldBeTrue)
			})
		})

		Convey("When a measurement is not finalized", func() {
			unfinalized := data.NewMeasurement(100, "BTC/USD", "kraken", 2, 43)
			recordBuilder := array.NewRecordBuilder(memory.DefaultAllocator, converted)
			defer recordBuilder.Release()

			err := fillMeasurements(recordBuilder, []*data.Measurement{unfinalized}, 100)
			So(err, ShouldNotBeNil)

			_, err = measurementRecords(schema, []*data.Measurement{unfinalized}, 100)
			So(err, ShouldNotBeNil)
		})
	})
}
