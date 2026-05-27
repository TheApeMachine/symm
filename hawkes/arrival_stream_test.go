package hawkes

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/trade"
)

func TestArrivalStreamFromTicks(t *testing.T) {
	now := time.Unix(1000, 0)
	windowStart := now.Add(-testFixtureWindow)

	ticks := []trade.Data{
		{Side: "buy", Timestamp: windowStart.Add(-time.Minute)},
		{Side: "buy", Timestamp: windowStart.Add(time.Minute)},
		{Side: "sell", Timestamp: now.Add(-time.Second)},
		{Side: "buy", Timestamp: now.Add(time.Minute)},
	}

	convey.Convey("Given ticks spanning a bounded measurement window", t, func() {
		stream := ArrivalStreamFromTicks(ticks, windowStart, now)

		convey.Convey("It should keep only in-window side events", func() {
			convey.So(len(stream.BuyTimes()), convey.ShouldEqual, 1)
			convey.So(len(stream.SellTimes()), convey.ShouldEqual, 1)
			convey.So(stream.BuyTimes()[0], convey.ShouldEqual, ticks[1].Timestamp)
		})
	})
}

func TestArrivalStreamMarked(t *testing.T) {
	start := time.Unix(0, 0)
	buyEvents := []time.Time{start.Add(3 * time.Second), start}
	sellEvents := []time.Time{start.Add(2 * time.Second)}

	convey.Convey("Given unsorted buy and sell timestamps", t, func() {
		marked := NewArrivalStream(buyEvents, sellEvents).Marked()

		convey.Convey("It should merge them chronologically", func() {
			convey.So(len(marked), convey.ShouldEqual, 3)
			convey.So(marked[0].at.Equal(start), convey.ShouldBeTrue)
			convey.So(marked[1].side, convey.ShouldEqual, sideSell)
			convey.So(marked[2].side, convey.ShouldEqual, sideBuy)
		})
	})
}
