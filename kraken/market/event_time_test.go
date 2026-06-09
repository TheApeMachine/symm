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
}
