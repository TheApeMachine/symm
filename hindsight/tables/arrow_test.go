package tables

import (
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
		measurement.From = at
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
			So(batch.NumCols(), ShouldEqual, 16)

			Convey("Then ReadMeasurements reconstructs the measurement faithfully 1:1", func() {
				read, err := ReadMeasurements(batch)
				So(err, ShouldBeNil)
				So(len(read), ShouldEqual, 1)

				reconstructed := read[0]
				So(reconstructed.ID, ShouldEqual, measurement.ID)
				So(reconstructed.Epoch, ShouldEqual, 100)
				So(reconstructed.Label, ShouldEqual, "BTC/USD")
				So(reconstructed.Source, ShouldEqual, "kraken")
				So(reconstructed.SeqIdx, ShouldEqual, 1)
				So(reconstructed.Tick, ShouldEqual, 42)
				So(reconstructed.At.Equal(at), ShouldBeTrue)
				So(reconstructed.From.Equal(at), ShouldBeTrue)
				So(reconstructed.Meta("side"), ShouldEqual, "bid")
				So(reconstructed.Meta("type"), ShouldEqual, "limit")
				So(reconstructed.Coherence(), ShouldEqual, measurement.Coherence())
				So(reconstructed.Maturity(), ShouldEqual, measurement.Maturity())

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

		Convey("When a stored measurement row carries an invalid timestamp", func() {
			invalid := data.NewMeasurement(100, "BTC/USD", "depthflow", 3, 44)
			// Leave At as zero time.Time{}
			invalid.From = at
			invalid.Write(
				data.NewMetric("book_imbalance", 0.1, data.UnitRatio, data.TimescaleInstantaneous),
			)

			recordBuilder := array.NewRecordBuilder(memory.DefaultAllocator, converted)
			defer recordBuilder.Release()

			So(fillMeasurements(recordBuilder, []*data.Measurement{invalid}, 100), ShouldBeNil)
			batch := recordBuilder.NewRecordBatch()
			defer batch.Release()

			Convey("Then ReadMeasurements halts and returns validation error", func() {
				read, err := ReadMeasurements(batch)
				So(err, ShouldNotBeNil)
				So(read, ShouldBeNil)
				So(strings.Contains(err.Error(), "invalid"), ShouldBeTrue)
				So(strings.Contains(err.Error(), "at is required"), ShouldBeTrue)
			})
		})
	})
}
