package tests_test

import (
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests"
	bookfixture "github.com/theapemachine/symm/tests/fixtures/book"
	tickerfixture "github.com/theapemachine/symm/tests/fixtures/ticker"
	tradefixture "github.com/theapemachine/symm/tests/fixtures/trade"
)

func TestTradeAggression(t *testing.T) {
	Convey("Given a trade UPDATE timeline", t, func() {
		frames := tests.TradeAggression(
			tests.FramesOf(tradefixture.NewFixture(tradefixture.UPDATE, 8)),
			4,
			5,
		)

		Convey("When aggression begins", func() {
			index := 0
			sawBuy := false
			var qty float64

			for frame := range frames {
				var payload map[string]any
				So(sonic.Unmarshal(frame.Payload, &payload), ShouldBeNil)
				row := payload["data"].([]any)[0].(map[string]any)

				if index >= 4 {
					So(row["side"], ShouldEqual, "buy")
					sawBuy = true
					qty = row["qty"].(float64)
				}

				index++
			}

			Convey("Then later trades are buy-side and size-scaled", func() {
				So(sawBuy, ShouldBeTrue)
				So(qty, ShouldBeGreaterThan, 0)
			})
		})
	})
}

func TestBookDecay(t *testing.T) {
	Convey("Given repeated two-sided book snapshots", t, func() {
		firstQty := 0.0
		lastQty := 0.0
		index := 0

		for frame := range tests.BookDecay(
			tests.Repeat(tests.FramesOf(bookfixture.NewFixture(bookfixture.SNAPSHOT, 1)), 8),
			0,
			0.9,
		) {
			var payload map[string]any
			So(sonic.Unmarshal(frame.Payload, &payload), ShouldBeNil)
			row := payload["data"].([]any)[0].(map[string]any)
			bid := row["bids"].([]any)[0].(map[string]any)
			qty := bid["qty"].(float64)

			if index == 0 {
				firstQty = qty
			}

			lastQty = qty
			index++
		}

		Convey("Then resting bid qty thins across the decay window", func() {
			So(lastQty, ShouldBeLessThan, firstQty)
		})
	})
}

func TestBookImbalance(t *testing.T) {
	Convey("Given repeated two-sided book snapshots", t, func() {
		var bidQty, askQty float64

		for frame := range tests.BookImbalance(
			tests.Repeat(tests.FramesOf(bookfixture.NewFixture(bookfixture.SNAPSHOT, 1)), 4),
			0,
			4,
			0.25,
		) {
			var payload map[string]any
			So(sonic.Unmarshal(frame.Payload, &payload), ShouldBeNil)
			row := payload["data"].([]any)[0].(map[string]any)
			bidQty = row["bids"].([]any)[0].(map[string]any)["qty"].(float64)
			askQty = row["asks"].([]any)[0].(map[string]any)["qty"].(float64)
		}

		Convey("Then the book is bid-loaded versus asks", func() {
			So(bidQty, ShouldBeGreaterThan, askQty)
		})
	})
}

func TestBidFlicker(t *testing.T) {
	Convey("Given a ticker UPDATE timeline", t, func() {
		priorBid := 0.0
		priorLast := 0.0
		flickeredBid := 0.0
		flickeredLast := 0.0
		index := 0

		for frame := range tests.BidFlicker(
			tests.FramesOf(tickerfixture.NewFixture(tickerfixture.UPDATE, 6)),
			2,
			[]float64{0.985},
		) {
			var payload map[string]any
			So(sonic.Unmarshal(frame.Payload, &payload), ShouldBeNil)
			row := payload["data"].([]any)[0].(map[string]any)
			bid := row["bid"].(float64)
			last := row["last"].(float64)

			if index == 1 {
				priorBid = bid
				priorLast = last
			}

			if index == 2 {
				flickeredBid = bid
				flickeredLast = last
			}

			index++
		}

		Convey("When BidFlicker applies after at", func() {
			Convey("Then bid drops relative to last while last keeps engine drift", func() {
				So(flickeredBid, ShouldBeLessThan, priorBid)
				So(flickeredBid/flickeredLast, ShouldBeLessThan, priorBid/priorLast)
				So(flickeredLast, ShouldBeGreaterThan, 0)
			})
		})
	})
}

func TestTouchCancel(t *testing.T) {
	Convey("Given repeated two-sided book snapshots", t, func() {
		firstQty := 0.0
		cancelledQty := 0.0
		index := 0

		for frame := range tests.TouchCancel(
			tests.Repeat(
				tests.FramesOf(bookfixture.NewFixture(bookfixture.SNAPSHOT, 1)),
				6,
			),
			2,
			0.001,
		) {
			var payload map[string]any
			So(sonic.Unmarshal(frame.Payload, &payload), ShouldBeNil)
			row := payload["data"].([]any)[0].(map[string]any)
			qty := row["bids"].([]any)[0].(map[string]any)["qty"].(float64)

			if index == 0 {
				firstQty = qty
			}

			if index == 2 {
				cancelledQty = qty
			}

			index++
		}

		Convey("When TouchCancel collapses the best bid", func() {
			Convey("Then resting qty thins without wiping the book", func() {
				So(cancelledQty, ShouldBeLessThan, firstQty*0.01)
				So(cancelledQty, ShouldBeGreaterThan, 0)
			})
		})
	})
}
