package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/public"
)

func TestDecodeTrades(t *testing.T) {
	Convey("Given a trade update envelope", t, func() {
		message := &public.SocketMessage{
			Channel: public.TradesChannel,
			Type:    "update",
			Data:    []byte(`[{"symbol":"BTC/EUR","price":1,"qty":0.1}]`),
		}

		Convey("It should decode every row", func() {
			rows, err := DecodeTrades(message)

			So(err, ShouldBeNil)
			So(len(rows), ShouldEqual, 1)
			So(rows[0].Symbol, ShouldEqual, "BTC/EUR")
			So(rows[0].Price, ShouldEqual, 1)
		})
	})
}

func TestDecodeTickers(t *testing.T) {
	Convey("Given a ticker update envelope", t, func() {
		message := &public.SocketMessage{
			Channel: public.TickerChannel,
			Type:    "update",
			Data:    []byte(`[{"symbol":"BTC/EUR","bid":1,"ask":2,"last":1.5}]`),
		}

		Convey("It should decode every row", func() {
			rows, err := DecodeTickers(message)

			So(err, ShouldBeNil)
			So(len(rows), ShouldEqual, 1)
			So(rows[0].Symbol, ShouldEqual, "BTC/EUR")
			So(rows[0].Last, ShouldEqual, 1.5)
		})
	})
}

func TestDecodeBooks(t *testing.T) {
	Convey("Given a book update envelope", t, func() {
		message := &public.SocketMessage{
			Channel: public.BookChannel,
			Type:    "update",
			Data:    []byte(`[{"symbol":"BTC/EUR","bids":[],"asks":[]}]`),
		}

		Convey("It should decode every row and stamp the envelope type", func() {
			rows, err := DecodeBooks(message)

			So(err, ShouldBeNil)
			So(len(rows), ShouldEqual, 1)
			So(rows[0].Symbol, ShouldEqual, "BTC/EUR")
			So(rows[0].Type, ShouldEqual, "update")
		})
	})
}
