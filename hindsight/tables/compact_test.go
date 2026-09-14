package tables_test

import (
	"context"
	"testing"
	"time"

	"github.com/apache/iceberg-go"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/tests/tablestest"
)

func TestCompactTable(t *testing.T) {
	Convey("Given an Iceberg table with multiple micro-appends", t, func() {
		ctx := context.Background()
		catalog := tablestest.New(t)
		writer := tables.NewWriter(catalog)

		now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

		// Create 3 separate append commits to produce 3 small data files
		for i := int64(1); i <= 3; i++ {
			writer.AddSpotTicker(tables.SpotTickerRow{
				Epoch:      1,
				Tick:       i,
				Symbol:     "BTC/USD",
				VenueAt:    now.Add(time.Duration(i) * time.Second),
				ReceivedAt: now.Add(time.Duration(i) * time.Second),
				Bid:        50000.0,
				BidQty:     1.0,
				Ask:        50001.0,
				AskQty:     1.0,
				Last:       50000.5,
			})

			err := writer.Commit(ctx)
			So(err, ShouldBeNil)
		}

		tbl, err := catalog.Load(ctx, tables.SpotTicker)
		So(err, ShouldBeNil)

		tasks, err := tbl.Scan().PlanFiles(ctx)
		So(err, ShouldBeNil)
		So(len(tasks), ShouldEqual, 3)

		Convey("When CompactTable is executed", func() {
			result, err := catalog.CompactTable(ctx, tables.SpotTicker, 64*1024*1024)
			So(err, ShouldBeNil)
			So(result.RewrittenFiles, ShouldEqual, 3)
			So(result.NewFiles, ShouldEqual, 1)

			Convey("Then the table contains consolidated data files with identical row data", func() {
				tblAfter, err := catalog.Load(ctx, tables.SpotTicker)
				So(err, ShouldBeNil)

				tasksAfter, err := tblAfter.Scan().PlanFiles(ctx)
				So(err, ShouldBeNil)
				So(len(tasksAfter), ShouldEqual, 1)

				tickers, err := catalog.SpotTickerScan(ctx, 1, iceberg.AlwaysTrue{}, 0)
				So(err, ShouldBeNil)
				So(len(tickers), ShouldEqual, 3)
				So(tickers[0].Tick, ShouldEqual, 1)
				So(tickers[1].Tick, ShouldEqual, 2)
				So(tickers[2].Tick, ShouldEqual, 3)
			})
		})

		Convey("When ExpireTableSnapshots is executed", func() {
			err := catalog.ExpireTableSnapshots(ctx, tables.SpotTicker, 1)
			So(err, ShouldBeNil)
		})
	})
}
