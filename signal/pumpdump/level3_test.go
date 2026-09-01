package pumpdump

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func pumpdumpOrder(price, qty float64, at time.Time) kraken.Level3Order {
	return kraken.Level3Order{
		Event:      "add",
		OrderID:    "order",
		LimitPrice: decimal.NewFromFloat64(price),
		OrderQty:   decimal.NewFromFloat64(qty),
		Timestamp:  at,
	}
}

func pumpdumpMessage(symbol string, at time.Time, bids, asks []kraken.Level3Order) kraken.Level3Data {
	return kraken.Level3Data{
		Symbol:    symbol,
		Timestamp: at,
		Bids:      bids,
		Asks:      asks,
	}
}

func TestLevel3Step(t *testing.T) {
	Convey("Given a message with an executable touch", t, func() {
		entity := NewLevel3()
		at := time.Unix(1_700_000_000, 0)

		Convey("Step derives the touch from the message's own orders", func() {
			measurement := entity.Step(pumpdumpMessage("BTC/USD", at,
				[]kraken.Level3Order{pumpdumpOrder(99, 1, at)},
				[]kraken.Level3Order{pumpdumpOrder(101, 1, at)},
			))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["best_bid"].Raw, ShouldEqual, 99.0)
			So(measurement.Metrics["best_ask"].Raw, ShouldEqual, 101.0)
			So(measurement.Metrics["midpoint"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["spread"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["relative_spread"].Raw, ShouldAlmostEqual, 0.02, 1e-12)

			// A stateless direct measurement is whole.
			So(measurement.Maturity, ShouldEqual, 1.0)
		})
	})

	Convey("Given a symbol whose book has never shown both sides", t, func() {
		entity := NewLevel3()
		at := time.Unix(1_700_000_000, 0)

		Convey("Step yields no measurement rather than an error", func() {
			// An incomplete touch is the normal opening state of an
			// incremental feed, not a failure: an Err here would be logged
			// as a hard error for every symbol on every startup.
			So(entity.Step(pumpdumpMessage("MISSING", at, nil, nil)), ShouldBeNil)
		})

		Convey("A one-sided message alone still yields no measurement", func() {
			So(entity.Step(pumpdumpMessage("ONESIDED", at,
				[]kraken.Level3Order{pumpdumpOrder(99, 1, at)},
				nil,
			)), ShouldBeNil)
		})
	})

	Convey("Given a symbol that has seen both sides across separate messages", t, func() {
		entity := NewLevel3()
		at := time.Unix(1_700_000_000, 0)

		// Kraken sends Level-3 as one-sided incremental updates: only the
		// side that changed is carried. The touch must be retained across
		// messages or nearly every update is discarded.
		So(entity.Step(pumpdumpMessage("BTC/USD", at,
			[]kraken.Level3Order{pumpdumpOrder(99, 1, at)},
			nil,
		)), ShouldBeNil)

		Convey("A later one-sided update borrows the retained opposite side", func() {
			measurement := entity.Step(pumpdumpMessage("BTC/USD", at.Add(time.Second),
				nil,
				[]kraken.Level3Order{pumpdumpOrder(101, 1, at.Add(time.Second))},
			))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["best_bid"].Raw, ShouldEqual, 99.0)
			So(measurement.Metrics["best_ask"].Raw, ShouldEqual, 101.0)
			So(measurement.Metrics["midpoint"].Raw, ShouldEqual, 100.0)
		})

		Convey("A better touch on one side supersedes the retained one", func() {
			second := at.Add(time.Second)

			measurement := entity.Step(pumpdumpMessage("BTC/USD", second,
				[]kraken.Level3Order{pumpdumpOrder(99.5, 1, second)},
				[]kraken.Level3Order{pumpdumpOrder(100.5, 1, second)},
			))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["best_bid"].Raw, ShouldEqual, 99.5)
			So(measurement.Metrics["best_ask"].Raw, ShouldEqual, 100.5)
		})

		Convey("An order behind the touch does not move it", func() {
			// A message announces orders; it does not restate the side. 98 is
			// worse than the resting 99, so the best bid is unchanged.
			second := at.Add(time.Second)

			measurement := entity.Step(pumpdumpMessage("BTC/USD", second,
				[]kraken.Level3Order{pumpdumpOrder(98, 1, second)},
				[]kraken.Level3Order{pumpdumpOrder(102, 1, second)},
			))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["best_bid"].Raw, ShouldEqual, 99.0)
			So(measurement.Metrics["best_ask"].Raw, ShouldEqual, 102.0)
		})

		Convey("Retained touches are keyed per symbol", func() {
			So(entity.Step(pumpdumpMessage("ETH/USD", at.Add(time.Second),
				nil,
				[]kraken.Level3Order{pumpdumpOrder(101, 1, at.Add(time.Second))},
			)), ShouldBeNil)
		})
	})
}

/*
TestLevel3Step_CrossedTouchRetainsFreshPrice pins the crossed-touch contract.
A crossed touch must never publish a spread — an inverted book has no spread to
report. But this feed is depth-limited (market.l3_depth) and arrives one side at
a time, so a fresh price crossing the OTHER side's RETAINED price is a normal
transient, not a crossed book: the two prices never coexisted on the wire.

Failing that frame discarded the fresh price and kept the stale one, so every
later spread was measured against a price nobody was quoting. The frame
therefore commits and reports nothing, and the next message on the lagging side
resolves the touch.
*/
func TestLevel3Step_CrossedTouchRetainsFreshPrice(t *testing.T) {
	Convey("Given a symbol whose retained touch would be crossed", t, func() {
		entity := NewLevel3()
		at := time.Unix(1_700_000_000, 0)
		second := at.Add(time.Second)
		third := second.Add(time.Second)

		So(entity.Step(pumpdumpMessage("BTC/USD", at,
			[]kraken.Level3Order{pumpdumpOrder(101, 1, at)},
			nil,
		)), ShouldBeNil)

		Convey("Completing the touch with a lower ask publishes no spread", func() {
			measurement := entity.Step(pumpdumpMessage("BTC/USD", second,
				nil,
				[]kraken.Level3Order{pumpdumpOrder(99, 1, second)},
			))

			So(measurement, ShouldBeNil)

			Convey("but the fresh ask is retained, not discarded", func() {
				// The bid catches up to 98: the true book is now 98 x 99.
				resolved := entity.Step(pumpdumpMessage("BTC/USD", third,
					[]kraken.Level3Order{pumpdumpOrder(98, 1, third)},
					nil,
				))

				So(resolved, ShouldNotBeNil)
				So(resolved.Err, ShouldBeNil)
				So(resolved.Metrics["best_bid"].Raw, ShouldEqual, 98.0)
				So(resolved.Metrics["best_ask"].Raw, ShouldEqual, 99.0)
				So(resolved.Metrics["spread"].Raw, ShouldEqual, 1.0)
			})
		})
	})
}

/*
TestLevel3Step_DeleteIsNotRestingLiquidity pins that a delete event describes
liquidity being REMOVED. Its price is not a quote, and because a delete can be
priced anywhere — including through the opposite side's retained touch —
treating it as the touch also manufactures a crossed book out of a healthy one.
*/
func TestLevel3Step_DeleteIsNotRestingLiquidity(t *testing.T) {
	Convey("Given a completed touch", t, func() {
		entity := NewLevel3()
		at := time.Unix(1_700_000_000, 0)
		second := at.Add(time.Second)

		first := entity.Step(pumpdumpMessage("BTC/USD", at,
			[]kraken.Level3Order{pumpdumpOrder(99, 1, at)},
			[]kraken.Level3Order{pumpdumpOrder(101, 1, at)},
		))

		So(first, ShouldNotBeNil)
		So(first.Err, ShouldBeNil)

		Convey("A delete does not become the touch", func() {
			removed := pumpdumpOrder(101, 1, second)
			removed.Event = "delete"

			measurement := entity.Step(pumpdumpMessage("BTC/USD", second,
				nil,
				[]kraken.Level3Order{removed},
			))

			// The message carried no resting order at all, so it says nothing
			// about either side and the retained touch is unchanged.
			So(measurement, ShouldBeNil)
		})
	})
}
