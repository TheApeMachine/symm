package broker

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestQuoteSlotObserveVolatility(t *testing.T) {
	convey.Convey("Given a quote cache with a distinct price path", t, func() {
		cache := NewQuoteCache(context.Background(), nil)

		cache.InstallQuoteForTest(Quote{Symbol: "BTC/EUR", Bid: 100, Ask: 100, Last: 100})
		cache.InstallQuoteForTest(Quote{Symbol: "BTC/EUR", Bid: 101, Ask: 101, Last: 101})
		cache.InstallQuoteForTest(Quote{Symbol: "BTC/EUR", Bid: 101, Ask: 101, Last: 101})
		cache.InstallQuoteForTest(Quote{Symbol: "BTC/EUR", Bid: 102, Ask: 102, Last: 102})

		convey.Convey("It should publish realized volatility on the snapshot", func() {
			quote, ok := cache.Snapshot("BTC/EUR")

			convey.So(ok, convey.ShouldBeTrue)
			convey.So(quote.Volatility, convey.ShouldBeGreaterThan, 0)
		})
	})
}
