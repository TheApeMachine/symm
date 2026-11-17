package broker

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/venue/kraken"
)

func historyTrade(id, order, pair, side, vol string, sec int64) kraken.HistoryTrade {
	volume := decimal.RequireFromString(vol)
	price := decimal.RequireFromString("1")

	return kraken.HistoryTrade{
		ID:      id,
		OrderID: order,
		Pair:    pair,
		Type:    side,
		Volume:  &volume,
		Price:   &price,
		Time:    time.Unix(sec, 0).UTC(),
	}
}

func TestLedgerFold(t *testing.T) {
	Convey("Setup", t, func() {
		ledger := NewLedger("real", nil, nil, nil)

		Convey("When one pair opens and closes twice", func() {
			_, closed := ledger.Fold(context.Background(), []kraken.HistoryTrade{
				historyTrade("t1", "o1", "XBTUSD", "buy", "1", 1),
				historyTrade("t2", "o2", "XBTUSD", "sell", "1", 2),
				historyTrade("t3", "o3", "XBTUSD", "buy", "2", 3),
				historyTrade("t4", "o4", "XBTUSD", "sell", "2", 4),
			})

			Convey("Each cycle is its own closed holding", func() {
				So(len(closed), ShouldEqual, 2)
				So(closed[0].ID, ShouldNotEqual, closed[1].ID)
				So(len(closed[0].Fills), ShouldEqual, 2)
				So(len(closed[1].Fills), ShouldEqual, 2)
				So(closed[0].EntryVenueOrderID, ShouldEqual, "o1")
				So(closed[0].ExitVenueOrderID, ShouldEqual, "o2")
				So(closed[1].EntryVenueOrderID, ShouldEqual, "o3")
				So(closed[1].ClosedAt, ShouldNotBeNil)
			})
		})

		Convey("When two buys flatten on one sell", func() {
			_, closed := ledger.Fold(context.Background(), []kraken.HistoryTrade{
				historyTrade("t2", "o1", "XBTUSD", "buy", "0.4", 2),
				historyTrade("t1", "o1", "XBTUSD", "buy", "0.6", 1),
				historyTrade("t3", "o2", "XBTUSD", "sell", "1", 3),
			})

			Convey("Partial prints stay on one holding", func() {
				So(len(closed), ShouldEqual, 1)
				So(len(closed[0].Fills), ShouldEqual, 3)
				So(closed[0].Qty.String(), ShouldEqual, "0")
				So(closed[0].Fills[0].ExecID, ShouldEqual, "t1")
			})
		})

		Convey("When a sell arrives before any buy", func() {
			open, closed := ledger.Fold(context.Background(), []kraken.HistoryTrade{
				historyTrade("t1", "o1", "XBTUSD", "sell", "1", 1),
				historyTrade("t2", "o2", "XBTUSD", "buy", "1", 2),
			})

			Convey("The orphan sell is ignored and the buy stays open", func() {
				So(len(closed), ShouldEqual, 0)
				So(len(open), ShouldEqual, 1)
				So(open[0].Qty.String(), ShouldEqual, "1")
				So(len(open[0].Fills), ShouldEqual, 1)
			})
		})

		Convey("When a sell only partly covers the buy", func() {
			open, closed := ledger.Fold(context.Background(), []kraken.HistoryTrade{
				historyTrade("t1", "o1", "ZECUSD", "buy", "1", 1),
				historyTrade("t2", "o2", "ZECUSD", "sell", "0.4", 2),
			})

			Convey("The holding stays open for the remainder", func() {
				So(len(closed), ShouldEqual, 0)
				So(len(open), ShouldEqual, 1)
				So(open[0].Qty.String(), ShouldEqual, "0.6")
				So(open[0].ClosedAt, ShouldBeNil)
			})
		})
	})
}

func BenchmarkLedgerFold(b *testing.B) {
	ledger := NewLedger("real", nil, nil, nil)
	trades := []kraken.HistoryTrade{
		historyTrade("t1", "o1", "XBTUSD", "buy", "1", 1),
		historyTrade("t2", "o2", "XBTUSD", "sell", "1", 2),
		historyTrade("t3", "o3", "XBTUSD", "buy", "2", 3),
		historyTrade("t4", "o4", "XBTUSD", "sell", "2", 4),
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = ledger.Fold(context.Background(), trades)
	}
}
