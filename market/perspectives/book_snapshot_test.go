package perspectives

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
)

func TestAttachBook(t *testing.T) {
	Convey("Given a live book snapshot", t, func() {
		book := market.Book{
			Bids: []market.BookLevel{{Price: 99, Qty: 2}, {Price: 98, Qty: 3}},
			Asks: []market.BookLevel{{Price: 101, Qty: 1}, {Price: 102, Qty: 4}},
		}

		measurement := AttachBook(Measurement{Symbol: "BTC/EUR", Last: 100}, 99, 101, 100, book, 2)

		Convey("It should copy depth and spread", func() {
			So(measurement.HasBookDepth(), ShouldBeTrue)
			So(len(measurement.BookBids), ShouldEqual, 2)
			So(len(measurement.BookAsks), ShouldEqual, 2)
			So(measurement.SpreadBPS, ShouldAlmostEqual, 200, 1)
		})
	})
}
