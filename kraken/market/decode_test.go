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
			So(rows[0].Type, ShouldEqual, "update")
		})
	})

	Convey("Given a multi-row trade envelope", t, func() {
		message := &public.SocketMessage{
			Channel: public.TradesChannel,
			Type:    "snapshot",
			Data: []byte(`[
				{"symbol":"BTC/EUR","price":1,"qty":0.1},
				{"symbol":"ETH/EUR","price":2,"qty":0.2}
			]`),
		}

		Convey("It should decode and stamp each row", func() {
			rows, err := DecodeTrades(message)

			So(err, ShouldBeNil)
			So(len(rows), ShouldEqual, 2)
			So(rows[0].Symbol, ShouldEqual, "BTC/EUR")
			So(rows[0].Price, ShouldEqual, 1)
			So(rows[0].Type, ShouldEqual, "snapshot")
			So(rows[1].Symbol, ShouldEqual, "ETH/EUR")
			So(rows[1].Price, ShouldEqual, 2)
			So(rows[1].Type, ShouldEqual, "snapshot")
		})
	})

	Convey("Given invalid trade JSON", t, func() {
		message := &public.SocketMessage{
			Channel: public.TradesChannel,
			Type:    "update",
			Data:    []byte(`not-json`),
		}

		Convey("It should return an error", func() {
			rows, err := DecodeTrades(message)

			So(err, ShouldNotBeNil)
			So(rows, ShouldBeNil)
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
			So(rows[0].Type, ShouldEqual, "update")
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

	Convey("Given a multi-row book envelope", t, func() {
		message := &public.SocketMessage{
			Channel: public.BookChannel,
			Type:    "snapshot",
			Data: []byte(`[
				{"symbol":"BTC/EUR","bids":[],"asks":[]},
				{"symbol":"ETH/EUR","bids":[],"asks":[]}
			]`),
		}

		Convey("It should stamp envelope type on each row", func() {
			rows, err := DecodeBooks(message)

			So(err, ShouldBeNil)
			So(len(rows), ShouldEqual, 2)
			So(rows[0].Type, ShouldEqual, "snapshot")
			So(rows[1].Type, ShouldEqual, "snapshot")
		})
	})
}
