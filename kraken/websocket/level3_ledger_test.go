package websocket

import (
	"context"
	"fmt"
	"hash/crc32"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestLevel3LedgerApply proves SDK book arithmetic and checksum validation share
the same exact fixed-point wire values without routing money through float64.
*/
func TestLevel3LedgerApply(t *testing.T) {
	Convey("Given a snapshot whose checksum depends on trailing zeroes", t, func() {
		live := New(context.Background(), nil, true, Level3WebSocketURL)
		live.books.CreateBook("BTC/USD", 10)
		checksum := crc32.ChecksumIEEE([]byte("1010002000010000010000"))
		payload := fmt.Appendf(nil, `{
			"channel":"level3",
			"type":"snapshot",
			"data":[{
				"symbol":"BTC/USD",
				"checksum":%d,
				"bids":[{"order_id":"bid","limit_price":"100.000","order_qty":"1.0000","timestamp":"2026-01-01T00:00:00Z"}],
				"asks":[{"order_id":"ask","limit_price":"101.000","order_qty":"2.0000","timestamp":"2026-01-01T00:00:00Z"}]
			}]
		}`, checksum)

		Convey("When the exact ledger applies the complete frame", func() {
			err := live.level3Ledger.Apply(live.books, payload)
			managed := live.books.GetBook("BTC/USD")

			Convey("Then checksum text and decimal book values remain exact", func() {
				So(err, ShouldBeNil)
				So(live.level3Ledger.orders["BTC/USD"]["bid"].price, ShouldEqual, "100.000")
				So(live.level3Ledger.orders["BTC/USD"]["ask"].quantity, ShouldEqual, "2.0000")
				So(managed.BestBid().Price.String(), ShouldEqual, "100.000")
				So(managed.BestAsk().Quantity.String(), ShouldEqual, "2.0000")
			})
		})
	})
}
