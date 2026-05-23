package market

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestSubscribeParamsInstrument(t *testing.T) {
	convey.Convey("Given default subscribe params", t, func() {
		convey.Convey("It should target the instrument channel with snapshot", func() {
			params := SubscribeParams{}.Instrument()
			convey.So(params.Channel, convey.ShouldEqual, "instrument")
			convey.So(params.Snapshot, convey.ShouldBeTrue)
		})
	})
}

func TestSubscribeParamsTrades(t *testing.T) {
	convey.Convey("Given trade symbols", t, func() {
		convey.Convey("It should target the trade channel with snapshot", func() {
			params := SubscribeParams{}.Trades([]string{"BTC/EUR"})
			convey.So(params.Channel, convey.ShouldEqual, "trade")
			convey.So(params.Symbol, convey.ShouldResemble, []string{"BTC/EUR"})
			convey.So(params.Snapshot, convey.ShouldBeTrue)
		})
	})
}

func TestSubscribeParamsBook(t *testing.T) {
	convey.Convey("Given book symbols", t, func() {
		convey.Convey("It should target the book channel with depth and snapshot", func() {
			params := SubscribeParams{}.Book([]string{"BTC/EUR"}, 10)
			convey.So(params.Channel, convey.ShouldEqual, "book")
			convey.So(params.Depth, convey.ShouldEqual, 10)
			convey.So(params.Snapshot, convey.ShouldBeTrue)
		})
	})
}
