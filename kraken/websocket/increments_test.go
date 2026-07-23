package websocket

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestIncrementsStamp(t *testing.T) {
	Convey("Given instrument increments cached on the websocket", t, func() {
		cache := &Increments{}
		cache.Remember(kraken.NewInstrument([]byte(
			`{"channel":"instrument","type":"snapshot","data":{"pairs":[{` +
				`"symbol":"BTC/USD","quote":"USD","status":"online",` +
				`"price_increment":"0.1","tick_size":"0.1"}]}}`,
		)))

		Convey("When a book for that symbol is stamped", func() {
			book := kraken.NewBook([]byte(
				`{"channel":"book","type":"snapshot","data":[{` +
					`"symbol":"BTC/USD","bids":[{"price":"1.0","qty":1}],` +
					`"asks":[{"price":"1.1","qty":1}]}]}`,
			))
			err := cache.Stamp(book)

			Convey("Then every row carries the venue price increment", func() {
				So(err, ShouldBeNil)
				So(book.Data[0].PriceIncrement, ShouldNotBeNil)
				So(book.Data[0].PriceIncrement.Float64(), ShouldEqual, 0.1)
			})
		})

		Convey("When a book symbol is unknown", func() {
			book := kraken.NewBook([]byte(
				`{"channel":"book","type":"snapshot","data":[{` +
					`"symbol":"ETH/USD","bids":[],"asks":[]}]}`,
			))

			Convey("Then Stamp fails instead of emitting an incomplete book", func() {
				So(cache.Stamp(book), ShouldNotBeNil)
				So(book.Data[0].PriceIncrement, ShouldBeNil)
			})
		})
	})
}
