package broker

import (
	"math"
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
		state := NewMakerQueueState(quote, trading.Buy, 100, time.Now().UnixNano(), 0.01)

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

func TestTradeDepletesMakerQueue(t *testing.T) {
	Convey("Given a resting buy limit at 100", t, func() {
		Convey("It should deplete on sell aggression at or below the limit", func() {
			qty, ok := TradeDepletesMakerQueue(
				trading.Buy,
				100,
				market.TradeUpdate{Side: "sell", Price: 100, Qty: 0.5},
			)

			So(ok, ShouldBeTrue)
			So(qty, ShouldEqual, 0.5)
		})

		Convey("It should ignore trades above the buy limit", func() {
			_, ok := TradeDepletesMakerQueue(
				trading.Buy,
				100,
				market.TradeUpdate{Side: "sell", Price: 100.5, Qty: 1},
			)

			So(ok, ShouldBeFalse)
		})

		Convey("It should deplete a resting sell on buy aggression at or above the limit", func() {
			qty, ok := TradeDepletesMakerQueue(
				trading.Sell,
				100,
				market.TradeUpdate{Side: "buy", Price: 100, Qty: 1.25},
			)

			So(ok, ShouldBeTrue)
			So(qty, ShouldEqual, 1.25)
		})

		Convey("It should ignore invalid trade prints", func() {
			_, ok := TradeDepletesMakerQueue(
				trading.Buy,
				100,
				market.TradeUpdate{Side: "sell", Price: 0, Qty: 1},
			)

			So(ok, ShouldBeFalse)
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

		fillPrice, adverseBps := MakerRestingFillPrice(trading.Buy, 100, quote, trade, 0.01)

		Convey("It should charge adverse selection above the limit", func() {
			So(adverseBps, ShouldBeGreaterThan, 0)
			So(fillPrice, ShouldBeGreaterThan, 100)
		})
	})
}

func TestBookLevelQtyMatchesTickIndexAcrossFloatSources(t *testing.T) {
	Convey("Given engine and JSON prices that differ at float64 equality", t, func() {
		tickSize := 0.01
		limitPrice := 100.0
		bookPrice := math.Nextafter(100.0, 101.0)
		levels := []market.BookLevel{{Price: bookPrice, Qty: 4.5}}

		Convey("It should still resolve queue depth at the same tick", func() {
			So(limitPrice == bookPrice, ShouldBeFalse)
			So(BookLevelQty(levels, limitPrice, tickSize), ShouldEqual, 4.5)
		})
	})
}

func TestMakerBookDepletion(t *testing.T) {
	Convey("Given consecutive book snapshots at a resting buy limit", t, func() {
		previous := Quote{
			Book: market.Book{
				Bids: []market.BookLevel{{Price: 100, Qty: 2}},
			},
		}
		current := Quote{
			Book: market.Book{
				Bids: []market.BookLevel{{Price: 100, Qty: 0.5}},
			},
		}

		depletion := MakerBookDepletion(trading.Buy, 100, previous, current, 0.01)

		Convey("It should measure quantity removed at the limit level", func() {
			So(depletion, ShouldEqual, 1.5)
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
		_, _ = MakerRestingFillPrice(trading.Buy, 100, quote, trade, 0.01)
	}
}

func BenchmarkTradeDepletesMakerQueue(b *testing.B) {
	trade := market.TradeUpdate{Side: "sell", Price: 100, Qty: 1}

	for b.Loop() {
		_, _ = TradeDepletesMakerQueue(trading.Buy, 100, trade)
	}
}
