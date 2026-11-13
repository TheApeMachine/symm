package system

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewMarket(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewMarket is called", func() {
			market := NewMarket()

			Convey("venue defaults land from viper", func() {
				So(market.Model, ShouldEqual, "paper")
				So(market.QuoteCurrency, ShouldEqual, "USD")
				So(market.Balance, ShouldEqual, 200)
				So(market.Subscribe.Batch, ShouldEqual, 100)
				So(market.Subscribe.Pace, ShouldEqual, 500*time.Millisecond)
				So(market.Level3.Depth, ShouldEqual, 10)
				So(market.Level3.RateLimit, ShouldEqual, 200)
				So(market.Level3.RingCapacity, ShouldEqual, 8192)
				So(market.Level3.Record, ShouldBeTrue)
			})
		})
	})
}
