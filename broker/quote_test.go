package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestQuoteFillPrice(t *testing.T) {
	Convey("Given a complete quote", t, func() {
		quote := Quote{Last: 100, Bid: 99, Ask: 101}

		buyPrice, err := quote.FillPrice("buy", 50)

		Convey("It should price a buy fill", func() {
			So(err, ShouldBeNil)
			So(buyPrice, ShouldBeGreaterThan, 100)
		})
	})
}

func TestQuoteComplete(t *testing.T) {
	Convey("Given only a last price", t, func() {
		last, bid, ask, err := (&Quote{Last: 100}).complete()

		Convey("It should derive bid and ask", func() {
			So(err, ShouldBeNil)
			So(last, ShouldEqual, 100)
			So(bid, ShouldEqual, 100)
			So(ask, ShouldEqual, 100)
		})
	})
}
