package toxicity

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
)

func TestNearTouchToxic(t *testing.T) {
	Convey("Given a near-touch toxic flag on the shared tracker", t, func() {
		ResetDefault()
		tracker := Default()
		now := time.Now()
		symbol := "BTC/EUR"
		price := 100.0

		tracker.ObserveMid(symbol, market.Pair{}, price)
		state := tracker.stateLocked(symbol, market.Pair{})
		state.bidTotal = 100

		tracker.ApplyOrder(symbol, market.Pair{}, "add", "order-1", SideBid, price, 15, now, now)
		tracker.ApplyOrder(symbol, market.Pair{}, "delete", "order-1", SideBid, price, 15, now, now)

		Convey("It should report near-touch toxicity for that symbol", func() {
			So(NearTouchToxic(symbol, now), ShouldBeTrue)
			So(NearTouchToxic("ETH/EUR", now), ShouldBeFalse)
		})
	})
}
