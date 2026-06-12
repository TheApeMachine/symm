package hawkes

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	hkernel "github.com/theapemachine/nomagique/hawkes"
	"github.com/theapemachine/symm/kraken/market"
)

func TestArrivalStreamFromTicks(t *testing.T) {
	t.Parallel()

	Convey("Given buy and sell ticks", t, func() {
		start := time.Unix(1_700_000_000, 0)
		stream := ArrivalStreamFromTicks(
			[]market.TradeUpdate{
				{Side: "buy", Timestamp: start},
				{Side: "sell", Timestamp: start.Add(time.Second)},
			},
			start,
			start.Add(2*time.Second),
		)

		Convey("It should partition sides", func() {
			So(len(stream.BuyTimes()), ShouldEqual, 1)
			So(len(stream.SellTimes()), ShouldEqual, 1)
		})
	})
}

func TestRevisionKey(t *testing.T) {
	t.Parallel()

	Convey("Given an arrival stream", t, func() {
		start := time.Unix(1_700_000_000, 0)
		stream := hkernel.NewArrivalStream(
			[]time.Time{start, start.Add(time.Second)},
			[]time.Time{start.Add(2 * time.Second)},
		)

		Convey("It should fingerprint buy and sell bounds", func() {
			key := revisionKey(stream)
			So(key.buyCount, ShouldEqual, 2)
			So(key.sellCount, ShouldEqual, 1)
			So(key.buyFirst, ShouldEqual, start.UnixNano())
		})
	})
}
