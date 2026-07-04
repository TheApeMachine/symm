package kraken

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewBookDataSlice(t *testing.T) {
	Convey("Given Kraken book data payloads", t, func() {
		payload := []byte(`[{
			"symbol": "MATIC/USD",
			"bids": [{"price": 0.5666, "qty": 4831.75496356}],
			"asks": [{"price": 0.5668, "qty": 4410.79769741}],
			"checksum": 2439117997,
			"timestamp": "2023-10-06T17:35:55.440295Z"
		}]`)

		books := NewBookDataSlice(payload)

		Convey("It should decode book levels and checksum", func() {
			So(len(books), ShouldEqual, 1)

			book := books[0]

			So(book.Symbol, ShouldEqual, "MATIC/USD")
			So(book.Checksum, ShouldEqual, uint32(2439117997))
			So(book.Bids[0].Price, ShouldAlmostEqual, 0.5666)
			So(book.Asks[0].Qty, ShouldAlmostEqual, 4410.79769741)
			So(book.Timestamp.IsZero(), ShouldBeFalse)
		})
	})
}
