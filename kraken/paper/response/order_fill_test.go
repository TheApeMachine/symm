package response

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
)

func TestTakerFillPriceWalksBook(t *testing.T) {
	Convey("Given an order handler with a deep ask book", t, func() {
		orders := &Orders{
			quotes: broker.EnsureQuoteCache(t.Context(), nil),
		}
		orders.quotes.InstallQuoteForTest(broker.Quote{
			Symbol: "BTC/EUR",
			Last:   100,
			Book: market.Book{
				Asks: []market.BookLevel{
					{Price: 100, Qty: 1},
					{Price: 102, Qty: 1},
				},
			},
		})

		Convey("It prices through broker.SlippageFill", func() {
			fill, err := orders.takerFillQuote("BTC/EUR", trading.Buy, 2, 0, reasoning.ActionNone)
			So(err, ShouldBeNil)
			So(fill.price, ShouldEqual, 101)
			So(fill.filledQty, ShouldEqual, 2)
		})
	})
}

func BenchmarkTakerFillPrice(b *testing.B) {
	orders := &Orders{
		quotes: broker.EnsureQuoteCache(b.Context(), nil),
	}
	orders.quotes.InstallQuoteForTest(broker.Quote{
		Symbol: "BTC/EUR",
		Last:   100,
		Book: market.Book{
			Asks: []market.BookLevel{
				{Price: 100, Qty: 1},
				{Price: 101, Qty: 1},
			},
		},
	})

	b.ReportAllocs()

	for b.Loop() {
		_, _ = orders.takerFillQuote("BTC/EUR", trading.Buy, 1.5, 0, reasoning.ActionNone)
	}
}
