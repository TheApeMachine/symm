package market

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic"
)

func TestEventTimeFromTrade(t *testing.T) {
	Convey("Given a trade with exchange timestamp", t, func() {
		at := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
		trade := &TradeUpdate{Symbol: "BTC/EUR", Timestamp: at}

		parsed, err := EventTimeFromTrade(trade)

		Convey("It should return the trade timestamp", func() {
			So(err, ShouldBeNil)
			So(parsed, ShouldEqual, at)
		})
	})

	Convey("Given a trade with zero timestamp", t, func() {
		_, err := EventTimeFromTrade(&TradeUpdate{Symbol: "BTC/EUR"})

		Convey("It should return an error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestEventTimeFromBus(t *testing.T) {
	Convey("Given a measurement with observed_at", t, func() {
		at := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
		measurement := logic.Measurement{
			Symbol:     "ETH/EUR",
			Source:     logic.SourceFluid,
			ObservedAt: at,
		}

		parsed, err := EventTimeFromBus("measurements", measurement)

		Convey("It should return observed_at", func() {
			So(err, ShouldBeNil)
			So(parsed, ShouldEqual, at)
		})
	})

	Convey("Given a level3 update with order timestamps", t, func() {
		earlier := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
		latest := time.Date(2024, 6, 1, 12, 0, 1, 0, time.UTC)
		update := &Level3Update{
			Symbol: "BTC/EUR",
			Bids: []Bid{
				{OrderID: "bid-1", Timestamp: earlier},
			},
			Asks: []Ask{
				{OrderID: "ask-1", Timestamp: latest},
			},
		}

		parsed, err := EventTimeFromBus("level3", update)

		Convey("It should return the latest order timestamp", func() {
			So(err, ShouldBeNil)
			So(parsed, ShouldEqual, latest)
		})
	})

	Convey("Given an empty level3 update", t, func() {
		_, err := EventTimeFromBus("level3", &Level3Update{Symbol: "BTC/EUR"})

		Convey("It should return an error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}
