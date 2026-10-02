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

func TestCatalog_Timeline(t *testing.T) {
	Convey("Given an Iceberg catalog with ticker, trade, and level3 records", t, func() {
		catalog := tablestest.New(t)
		ctx := context.Background()
		at := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
		epoch := int64(100)

		err := catalog.RecordRun(ctx, tables.Run{
			Epoch:     epoch,
			StartedAt: at,
			BuildID:   "test",
			Status:    "ACTIVE",
		})
		So(err, ShouldBeNil)

		writer := tables.NewWriter(catalog, epoch)

		ticker1 := data.NewMeasurement[float64]("spot_ticker", nil)
		ticker1.Label = "BTC/USD"
		ticker1.SeqIdx = 10
		ticker1.At = at
		ticker1.Maturity = 1.0
		ticker1.WriteMetric("bid", 50000)
		ticker1.WriteMetric("ask", 50001)
		ticker1.WriteMetric("last", 50000.5)
		writer.Add("ticker", data.Publication{Measurement: ticker1})

		trade1 := data.NewMeasurement[float64]("spot_trade", nil)
		trade1.Label = "BTC/USD"
		trade1.SeqIdx = 8
		trade1.At = at
		trade1.Maturity = 1.0
		trade1.WriteMetric("price", 50000.5)
		trade1.WriteMetric("qty", 0.5)
		writer.Add("trade", data.Publication{Measurement: trade1})

		l31 := data.NewMeasurement[float64]("spot_level3", nil)
		l31.Label = "BTC/USD"
		l31.SeqIdx = 9
		l31.At = at
		l31.Maturity = 1.0
		l31.WriteMetric("limit_price", 50000)
		l31.WriteMetric("order_qty", 1.0)
		writer.Add("level3", data.Publication{Measurement: l31})

		sig1 := data.NewMeasurement[float64]("cvd", nil)
		sig1.Label = "BTC/USD"
		sig1.SeqIdx = 10
		sig1.At = at
		sig1.Maturity = 1.0
		sig1.WriteMetric("delta", 100.0)
		writer.Add("measurements", data.Publication{Measurement: sig1})

		ticker2 := data.NewMeasurement[float64]("spot_ticker", nil)
		ticker2.Label = "BTC/USD"
		ticker2.SeqIdx = 20
		ticker2.At = at.Add(time.Second)
		ticker2.Maturity = 1.0
		ticker2.WriteMetric("bid", 50002)
		ticker2.WriteMetric("ask", 50003)
		ticker2.WriteMetric("last", 50002.5)
		writer.Add("ticker", data.Publication{Measurement: ticker2})

		trade2 := data.NewMeasurement[float64]("spot_trade", nil)
		trade2.Label = "BTC/USD"
		trade2.SeqIdx = 15
		trade2.At = at.Add(time.Second)
		trade2.Maturity = 1.0
		trade2.WriteMetric("price", 50002.0)
		trade2.WriteMetric("qty", 2.0)
		writer.Add("trade", data.Publication{Measurement: trade2})

		So(writer.CommitReady(ctx, true), ShouldBeNil)

		Convey("Timeline attaches concurrent trades, level3, and measurements to ticker.Peers", func() {
			var reconstructed []*data.Measurement[float64]

			for reading := range catalog.Timeline(ctx, epoch, "BTC/USD", 0, 0) {
				reconstructed = append(reconstructed, reading)
			}

			So(len(reconstructed), ShouldEqual, 2)

			// First ticker at SeqIdx=10 should have trade1 (SeqIdx=8), l31 (SeqIdx=9), sig1 (SeqIdx=10)
			So(reconstructed[0].SeqIdx, ShouldEqual, 10)
			So(len(reconstructed[0].Peers), ShouldEqual, 3)
			So(reconstructed[0].Peers[0].Source, ShouldEqual, "spot_trade")
			So(reconstructed[0].Peers[1].Source, ShouldEqual, "spot_level3")
			So(reconstructed[0].Peers[2].Source, ShouldEqual, "cvd")

			// Second ticker at SeqIdx=20 should have trade2 (SeqIdx=15)
			So(reconstructed[1].SeqIdx, ShouldEqual, 20)
			So(len(reconstructed[1].Peers), ShouldEqual, 1)
			So(reconstructed[1].Peers[0].Source, ShouldEqual, "spot_trade")
		})

		Convey("Symbols lists all distinct symbols in the epoch", func() {
			symbols, err := catalog.Symbols(ctx, epoch)
			So(err, ShouldBeNil)
			So(symbols, ShouldResemble, []string{"BTC/USD"})
		})
	})
}
