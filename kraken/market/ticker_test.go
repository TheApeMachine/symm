package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTickerResolvePrice(t *testing.T) {
	Convey("Given a ticker with last price", t, func() {
		price, err := (&TickerUpdate{Symbol: "BTC/EUR", Last: 42000}).ResolvePrice()

		Convey("It should return last", func() {
			So(err, ShouldBeNil)
			So(price, ShouldEqual, 42000)
		})
	})

	Convey("Given a ticker with bid and ask only", t, func() {
		price, err := (&TickerUpdate{Symbol: "GBP/USD", Ask: 1.26, Bid: 1.24}).ResolvePrice()

		Convey("It should return the mid price", func() {
			So(err, ShouldBeNil)
			So(price, ShouldAlmostEqual, 1.25, 1e-12)
		})
	})

	Convey("Given a ticker with no usable price", t, func() {
		_, err := (&TickerUpdate{Symbol: "GBP/USD"}).ResolvePrice()

		Convey("It should reject zero price", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkTickerResolvePrice(b *testing.B) {
	ticker := &TickerUpdate{Symbol: "BTC/EUR", Last: 42000, Ask: 42001, Bid: 41999}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = ticker.ResolvePrice()
	}
}
