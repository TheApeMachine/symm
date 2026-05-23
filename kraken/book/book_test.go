package book

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
)

func TestTopImbalance(t *testing.T) {
	convey.Convey("Given top-of-book bid and ask volumes", t, func() {
		top := market.BookTop{
			BestBid: market.BookLevel{Price: 100, Volume: 3},
			BestAsk: market.BookLevel{Price: 101, Volume: 1},
		}

		convey.Convey("It should compute bid-side imbalance", func() {
			imbalance := topImbalance(top)
			convey.So(imbalance, convey.ShouldEqual, 0.5)
		})
	})
}
