package kraken

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewBook(t *testing.T) {
	Convey("Given a Kraken book frame", t, func() {
		payload := []byte(`{"type":"update","data":[{"symbol":"MATIC/USD","bids":[{"price":0.5657,"qty":1098.3947558}],"asks":[{"price":0.5658,"qty":4410.79769741}],"checksum":2114181697,"timestamp":"2023-10-06T17:35:55.440295Z"}]}`)

		book := NewBook(payload)

		Convey("It should decode the envelope row directly", func() {
			So(len(book.Data), ShouldEqual, 1)
			So(book.Data[0].Type, ShouldEqual, "update")
			So(book.Data[0].Symbol, ShouldEqual, "MATIC/USD")
			So(book.Data[0].Checksum, ShouldEqual, uint32(2114181697))
			So(book.Data[0].Bids[0].Price.String(), ShouldEqual, "0.5657")
			So(book.Data[0].Asks[0].Qty, ShouldAlmostEqual, 4410.79769741)
			So(book.Data[0].Timestamp.IsZero(), ShouldBeFalse)
		})
	})
}
