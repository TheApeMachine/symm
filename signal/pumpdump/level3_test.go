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

		Convey("A newer touch on one side supersedes the retained one", func() {
			measurement := entity.Step(pumpdumpMessage("BTC/USD", at.Add(time.Second),
				[]kraken.Level3Order{pumpdumpOrder(98, 1, at.Add(time.Second))},
				[]kraken.Level3Order{pumpdumpOrder(102, 1, at.Add(time.Second))},
			))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["best_bid"].Raw, ShouldEqual, 98.0)
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

func TestLevel3Step_CrossedBookStillFailsClosed(t *testing.T) {
	Convey("Given a symbol whose retained touch would be crossed", t, func() {
		entity := NewLevel3()
		at := time.Unix(1_700_000_000, 0)

		// Moving PositiveOrder inside the touch-complete gate must not weaken
		// it: a crossed book still has to fail closed once both sides exist.
		So(entity.Step(pumpdumpMessage("BTC/USD", at,
			[]kraken.Level3Order{pumpdumpOrder(101, 1, at)},
			nil,
		)), ShouldBeNil)

		Convey("Completing the touch with a lower ask is rejected", func() {
			measurement := entity.Step(pumpdumpMessage("BTC/USD", at.Add(time.Second),
				nil,
				[]kraken.Level3Order{pumpdumpOrder(99, 1, at.Add(time.Second))},
			))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldNotBeNil)
		})
	})
}
