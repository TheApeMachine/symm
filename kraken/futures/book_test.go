package futures

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
)

func TestBookFromSnapshot(t *testing.T) {
	convey.Convey("Given a futures book snapshot frame", t, func() {
		convey.Convey("It should map to the shared L2 book with perpetual lane identity", func() {
			book, err := BookFromSnapshot(bookSnapshotMessage{
				Feed:      bookSnapshotFeed,
				ProductID: "PI_XBTUSD",
				Timestamp: 1_700_000_000_000,
				Bids:      []bookLevel{{Price: 50000, Qty: 1.5}},
				Asks:      []bookLevel{{Price: 50001, Qty: 2}},
			})

			convey.So(err, convey.ShouldBeNil)
			convey.So(book.IsSnapshot(), convey.ShouldBeTrue)
			convey.So(book.InstrumentIdentity().Lane, convey.ShouldEqual, market.InstrumentLanePerpetual)
			convey.So(book.InstrumentIdentity().Base, convey.ShouldEqual, "XBT")
			convey.So(len(book.Bids), convey.ShouldEqual, 1)
			convey.So(len(book.Asks), convey.ShouldEqual, 1)
		})
	})
}

func TestBookFromDelta(t *testing.T) {
	convey.Convey("Given a futures book delta frame", t, func() {
		convey.Convey("It should map to an incremental bid update", func() {
			book, err := BookFromDelta(bookDeltaMessage{
				Feed:      bookDeltaFeed,
				ProductID: "PI_XBTUSD",
				Side:      "buy",
				Price:     49999,
				Qty:       0.5,
				Timestamp: 1_700_000_000_100,
			})

			convey.So(err, convey.ShouldBeNil)
			convey.So(book.IsSnapshot(), convey.ShouldBeFalse)
			convey.So(len(book.Bids), convey.ShouldEqual, 1)
			convey.So(book.Bids[0].Price, convey.ShouldEqual, 49999)
		})
	})
}
