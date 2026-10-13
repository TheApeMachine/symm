package tables_test

import (
	"context"
	"testing"
	"time"

	"github.com/apache/iceberg-go"
	icetable "github.com/apache/iceberg-go/table"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/tests/tablestest"
)

func TestCatalog_Compact(t *testing.T) {
	Convey("Given an Iceberg catalog with multiple tiny files for an epoch", t, func() {
		ctx := context.Background()
		catalog := tablestest.New(t)
		epoch := int64(300)

		// Append 5 separate single-row measurements to simulate 5 tiny files.
		for index := int64(1); index <= 5; index++ {
			now := time.Now().UTC()
			measurement := data.NewMeasurement(epoch, "BTC/USD", "spot:trade", now, now)
			measurement.Write(data.NewMetric("price", 50000.0+float64(index), data.UnitCurrency, data.TimescaleTick))

			err := catalog.Append(ctx, tables.Measurements, epoch, []*data.Measurement{measurement})
			So(err, ShouldBeNil)
		}

		tbl, err := catalog.Load(ctx, tables.Measurements)
		So(err, ShouldBeNil)

		tasks, err := tbl.Scan(icetable.WithRowFilter(iceberg.EqualTo(iceberg.Reference("epoch"), epoch))).PlanFiles(ctx)
		So(err, ShouldBeNil)
		So(len(tasks), ShouldEqual, 5)

		Convey("When Compact is called for that epoch", func() {
			err := catalog.Compact(ctx, tables.Measurements, epoch)
			So(err, ShouldBeNil)

			tbl, err = catalog.Load(ctx, tables.Measurements)
			So(err, ShouldBeNil)

			compactedTasks, err := tbl.Scan(icetable.WithRowFilter(iceberg.EqualTo(iceberg.Reference("epoch"), epoch))).PlanFiles(ctx)
			So(err, ShouldBeNil)
			So(len(compactedTasks), ShouldEqual, 1)

			readMeasurements, err := drainSeq(catalog.Trades(ctx, epoch))
			So(err, ShouldBeNil)
			So(len(readMeasurements), ShouldEqual, 5)
		})
	})
}
