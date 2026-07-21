package websocket

import (
	"context"
	"fmt"
	"hash/crc32"
	"strconv"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
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

/*
TestLevel3Ledger_pruneDepth proves repeated best-price arrivals cannot grow the
exact checksum ledger beyond the orders retained by the subscribed SDK depth.
*/
func TestLevel3Ledger_pruneDepth(t *testing.T) {
	Convey("Given a three-level book receiving a long sequence of new bids", t, func() {
		manager := spot.NewBookManager()
		managed := manager.CreateBook("BTC/USD", 3)
		managed.EnableMaxDepth = false
		ledger := newLevel3Ledger()
		ledger.orders["BTC/USD"] = make(map[string]level3Order)
		quantity := decimal.NewFromInt64(1)

		for sequence := range 100 {
			orderID := "bid-" + strconv.Itoa(sequence)
			price := decimal.NewFromInt64(int64(100 + sequence))
			managed.Update(&book.UpdateOptions{
				Direction: book.Bid,
				ID:        orderID,
				Price:     price,
				Quantity:  quantity,
				Timestamp: time.Unix(int64(sequence), 0),
			})
			ledger.orders["BTC/USD"][orderID] = level3Order{
				price:    price.String(),
				quantity: "1",
			}
			ledger.pruneDepth(managed, "BTC/USD")
			managed.EnforceDepth()
		}

		Convey("Then SDK and exact ledgers retain the same bounded population", func() {
			So(managed.Bids.Levels, ShouldHaveLength, 3)
			So(ledger.orders["BTC/USD"], ShouldHaveLength, 3)
			So(managed.BestBid().Price.Float64(), ShouldEqual, 199.0)
			So(managed.WorstBid().Price.Float64(), ShouldEqual, 197.0)
		})
	})
}
