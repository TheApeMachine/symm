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

		ticker1 := &data.Measurement[float64]{
			Source:   "spot_ticker",
			Label:    "BTC/USD",
			SeqIdx:   10,
			At:       at,
			Maturity: 1.0,
			Metrics: map[string]data.Metric[float64]{
				"bid":  {Label: "bid", Raw: 50000},
				"ask":  {Label: "ask", Raw: 50001},
				"last": {Label: "last", Raw: 50000.5},
			},
		}
		writer.Add("ticker", ticker1)

		trade1 := &data.Measurement[float64]{
			Source:   "spot_trade",
			Label:    "BTC/USD",
			SeqIdx:   8,
			At:       at,
			Maturity: 1.0,
			Metrics: map[string]data.Metric[float64]{
				"price": {Label: "price", Raw: 50000.5},
				"qty":   {Label: "qty", Raw: 0.5},
			},
		}
		writer.Add("trade", trade1)

		l31 := &data.Measurement[float64]{
			Source:   "spot_level3",
			Label:    "BTC/USD",
			SeqIdx:   9,
			At:       at,
			Maturity: 1.0,
			Metrics: map[string]data.Metric[float64]{
				"limit_price": {Label: "limit_price", Raw: 50000},
				"order_qty":   {Label: "order_qty", Raw: 1.0},
			},
		}
		writer.Add("level3", l31)

		sig1 := &data.Measurement[float64]{
			Source:   "cvd",
			Label:    "BTC/USD",
			SeqIdx:   10,
			At:       at,
			Maturity: 1.0,
			Metrics: map[string]data.Metric[float64]{
				"delta": {Label: "delta", Raw: 100.0},
			},
		}
		writer.Add("measurements", sig1)

		ticker2 := &data.Measurement[float64]{
			Source:   "spot_ticker",
			Label:    "BTC/USD",
			SeqIdx:   20,
			At:       at.Add(time.Second),
			Maturity: 1.0,
			Metrics: map[string]data.Metric[float64]{
				"bid":  {Label: "bid", Raw: 50002},
				"ask":  {Label: "ask", Raw: 50003},
				"last": {Label: "last", Raw: 50002.5},
			},
		}
		writer.Add("ticker", ticker2)

		trade2 := &data.Measurement[float64]{
			Source:   "spot_trade",
			Label:    "BTC/USD",
			SeqIdx:   15,
			At:       at.Add(time.Second),
			Maturity: 1.0,
			Metrics: map[string]data.Metric[float64]{
				"price": {Label: "price", Raw: 50002.0},
				"qty":   {Label: "qty", Raw: 2.0},
			},
		}
		writer.Add("trade", trade2)

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
