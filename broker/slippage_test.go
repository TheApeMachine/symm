package broker

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestSlippageFillWalksBook(t *testing.T) {
	convey.Convey("Given an ask book with two levels", t, func() {
		quote := Quote{
			Symbol: "BTC/EUR",
			Bid:    99,
			Ask:    100,
			Last:   99.5,
			Book: market.Book{
				Asks: []market.BookLevel{
					{Price: 100, Qty: 1},
					{Price: 101, Qty: 1},
				},
			},
			UpdatedAt: time.Now().UTC(),
		}

		convey.Convey("It should VWAP through available depth", func() {
			fill, err := SlippageFill(quote, trading.Buy, 1.5)

			convey.So(err, convey.ShouldBeNil)
			convey.So(fill.Price, convey.ShouldAlmostEqual, 100.3333333333, 0.0000001)
			convey.So(fill.DepthCoverage, convey.ShouldEqual, 1)
		})
	})
}

func TestSlippageFillHalfSpread(t *testing.T) {
	convey.Convey("Given a quote without book depth", t, func() {
		quote := Quote{
			Symbol: "BTC/EUR",
			Bid:    99,
			Ask:    101,
			Last:   100,
		}

		convey.Convey("It should cross half the spread for buys and sells", func() {
			buyFill, err := SlippageFill(quote, trading.Buy, 1)

			convey.So(err, convey.ShouldBeNil)
			convey.So(buyFill.Price, convey.ShouldAlmostEqual, 101, 1e-9)
			convey.So(buyFill.SlippageBps, convey.ShouldAlmostEqual, 100, 1e-9)

			sellFill, err := SlippageFill(quote, trading.Sell, 1)

			convey.So(err, convey.ShouldBeNil)
			convey.So(sellFill.Price, convey.ShouldAlmostEqual, 99, 1e-9)
		})
	})
}

func TestWouldCrossPostOnly(t *testing.T) {
	convey.Convey("Given a live quote", t, func() {
		quote := Quote{Bid: 99, Ask: 100}

		convey.Convey("It should reject crossing buy limits", func() {
			convey.So(WouldCrossPostOnly(quote, trading.Buy, 100), convey.ShouldBeTrue)
			convey.So(WouldCrossPostOnly(quote, trading.Buy, 98), convey.ShouldBeFalse)
		})
	})
}

func BenchmarkSlippageFill(b *testing.B) {
	quote := Quote{
		Symbol: "BTC/EUR",
		Bid:    99,
		Ask:    100,
		Last:   99.5,
		Book: market.Book{
			Asks: []market.BookLevel{
				{Price: 100, Qty: 2},
				{Price: 101, Qty: 3},
			},
		},
		UpdatedAt: time.Now().UTC(),
	}

	for b.Loop() {
		_, _ = SlippageFill(quote, trading.Buy, 1)
	}
}
