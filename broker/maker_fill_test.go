package broker

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestMakerQueueState(t *testing.T) {
	Convey("Given resting queue ahead at the limit level", t, func() {
		quote := Quote{
			Book: market.Book{
				Bids: []market.BookLevel{{Price: 100, Qty: 2.5}},
			},
		}
		state := NewMakerQueueState(quote, trading.Buy, 100, time.Now().UnixNano())

		Convey("It should require trade depletion before filling", func() {
			So(state.QueueAhead, ShouldEqual, 2.5)
			So(state.Ready(), ShouldBeFalse)

			state.Deplete(1)

			So(state.Ready(), ShouldBeFalse)

			state.Deplete(2)

			So(state.Ready(), ShouldBeTrue)
		})
	})
}

func TestMakerRestingFillPrice(t *testing.T) {
	Convey("Given a sell-aggressor trade lifting a resting bid", t, func() {
		quote := Quote{
			Bid: 99.5,
			Ask: 100.5,
			Book: market.Book{
				Bids: []market.BookLevel{{Price: 100, Qty: 1}},
			},
		}
		trade := market.TradeUpdate{Side: "sell", Price: 100, Qty: 1}

		fillPrice, adverseBps := MakerRestingFillPrice(trading.Buy, 100, quote, trade)

		Convey("It should charge adverse selection above the limit", func() {
			So(adverseBps, ShouldBeGreaterThan, 0)
			So(fillPrice, ShouldBeGreaterThan, 100)
		})
	})
}

func TestReplayMakerAdverseSlippagePct(t *testing.T) {
	Convey("Given wide spread under stress", t, func() {
		slippage := ReplayMakerAdverseSlippagePct(20, 1.5)

		Convey("It should charge extra maker drag", func() {
			So(slippage, ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkMakerRestingFillPrice(b *testing.B) {
	quote := Quote{
		Bid: 99.5,
		Ask: 100.5,
		Book: market.Book{
			Bids: []market.BookLevel{{Price: 100, Qty: 2}},
		},
	}
	trade := market.TradeUpdate{Side: "sell", Price: 100, Qty: 1}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = MakerRestingFillPrice(trading.Buy, 100, quote, trade)
	}
}
