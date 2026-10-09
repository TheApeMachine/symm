package tables_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/tests/tablestest"
)

func TestWriter_AddAndCommit(t *testing.T) {
	Convey("Given an Iceberg catalog and a Writer", t, func() {
		ctx := context.Background()
		catalog := tablestest.New(t)
		epoch := int64(200)
		writer := tables.NewWriter(catalog, epoch)

		Convey("When no measurements have been added", func() {
			So(writer.BufferedRows(), ShouldEqual, 0)
			So(writer.BufferedBytes(), ShouldEqual, 0)

			writer.Add(tables.Measurements, nil)
			So(writer.BufferedRows(), ShouldEqual, 0)
		})

		Convey("When measurements are added to the buffer", func() {
			measurement := data.NewMeasurement(epoch, "ETH/USD", "spot:trade", 1, 1)
			measurement.At = time.Now().UTC()
			measurement.From = measurement.At
			measurement.Write(data.NewMetric("price", 3000.50, data.UnitCurrency, data.TimescaleTick))

			writer.Add(tables.Measurements, measurement)

			So(writer.BufferedRows(), ShouldEqual, 1)
			So(writer.BufferedBytes(), ShouldBeGreaterThan, 0)

			Convey("Then CommitReady with forceAll=false skips commit if below threshold", func() {
				So(writer.CommitReady(ctx, false), ShouldBeNil)
				So(writer.BufferedRows(), ShouldEqual, 1)
			})

			Convey("Then CommitReady with forceAll=true writes to table and clears buffer", func() {
				So(writer.CommitReady(ctx, true), ShouldBeNil)
				So(writer.BufferedRows(), ShouldEqual, 0)
				So(writer.BufferedBytes(), ShouldEqual, 0)

				readMeasurements, err := drainSeq(catalog.Trades(ctx, epoch))
				So(err, ShouldBeNil)
				So(len(readMeasurements), ShouldEqual, 1)
				So(readMeasurements[0].Label, ShouldEqual, "ETH/USD")
			})
		})

		Convey("When ReleaseRemaining is called", func() {
			measurement := data.NewMeasurement(epoch, "SOL/USD", "detector", 2, 2)
			measurement.At = time.Now().UTC()
			measurement.From = measurement.At
			writer.Add(tables.Measurements, measurement)

			So(writer.BufferedRows(), ShouldEqual, 1)
			writer.ReleaseRemaining()
			So(writer.BufferedRows(), ShouldEqual, 0)
			So(writer.BufferedBytes(), ShouldEqual, 0)
		})
	})
}
