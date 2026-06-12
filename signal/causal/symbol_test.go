package causal

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestCausalSymbolMeasureZeroChangePct(t *testing.T) {
	Convey("Given macro drift without change_pct", t, func() {
		state, err := NewCausalSymbol()
		So(err, ShouldBeNil)

		state.FeedTicker(krakenmarket.TickerUpdate{
			Symbol: "BTC/EUR",
			Last:   50000,
			Bid:    49990,
			Ask:    50010,
		})

		Convey("It should measure from macro momentum without requiring change_pct", func() {
			reading, err := state.Measure(0.02, 0.5, time.Now())

			So(err, ShouldBeNil)
			So(reading.Strength, ShouldBeGreaterThan, 0)
		})
	})
}

func TestCausalSymbolFeedBookOneSidedDelta(t *testing.T) {
	Convey("Given ticker-primed touch and a one-sided book delta", t, func() {
		state, err := NewCausalSymbol()
		So(err, ShouldBeNil)

		state.FeedTicker(krakenmarket.TickerUpdate{
			Symbol: "BTC/EUR",
			Last:   50000,
			Bid:    49990,
			Ask:    50010,
		})

		So(state.FeedBook(krakenmarket.BookUpdate{
			Bids: []krakenmarket.BookLevel{{Price: 49990, Qty: 10}},
			Asks: []krakenmarket.BookLevel{{Price: 50010, Qty: 10}},
		}), ShouldBeNil)

		Convey("It should accept a bid-only delta using the prior ask", func() {
			So(state.FeedBook(krakenmarket.BookUpdate{
				Bids: []krakenmarket.BookLevel{{Price: 49995, Qty: 8}},
			}), ShouldBeNil)
			So(state.bid, ShouldEqual, 49995)
			So(state.ask, ShouldEqual, 50010)
		})
	})
}
