package signal

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func init() {
	viper.Set("signals.feed_ring_capacity", 64)
}

func TestTradeWindow(testingTB *testing.T) {
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	Convey("Given a trade feed with a click clock window", testingTB, func() {
		tradeFeed := NewTrade(context.Background())
		tradeFeed.Update(TradeArtifact(
			TradeRecord{Symbol: "ETH/EUR", Price: 100, Qty: 1, Timestamp: eventAt},
			TradeRecord{Symbol: "ETH/EUR", Price: 101, Qty: 2, Timestamp: eventAt.Add(time.Millisecond)},
			TradeRecord{Symbol: "ETH/EUR", Price: 102, Qty: 3, Timestamp: eventAt.Add(2 * time.Millisecond)},
		))

		window, ok := tradeFeed.Window("ETH/EUR")

		Convey("It should expose the scoped price window", func() {
			So(ok, ShouldBeTrue)
			So(len(window.Prices), ShouldEqual, 3)
			So(window.Volume, ShouldEqual, 6)
			So(window.Latest.Price, ShouldEqual, 102)
		})
	})
}

func TestBookSpread(testingTB *testing.T) {
	Convey("Given a book feed with touch quotes", testingTB, func() {
		bookFeed := NewBook(context.Background())
		bookFeed.Update(BookArtifact(BookRecord{
			Symbol: "BTC/EUR",
			Bids:   []BookLevelRecord{{Price: 100, Qty: 1}},
			Asks:   []BookLevelRecord{{Price: 101, Qty: 1}},
		}))

		Convey("It should compute spread in basis points", func() {
			So(bookFeed.Spread("BTC/EUR"), ShouldAlmostEqual, 99.50248756218905, 0.0001)
		})
	})
}
