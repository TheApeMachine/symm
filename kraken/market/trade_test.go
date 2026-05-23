package market

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestParseTrades(t *testing.T) {
	convey.Convey("Given a Kraken v2 trade websocket frame", t, func() {
		convey.Convey("It should extract every execution in the data array", func() {
			ticks, err := ParseTrades(sampleTradeFrame)
			convey.So(err, convey.ShouldBeNil)
			convey.So(len(ticks), convey.ShouldEqual, 2)
			convey.So(ticks[0].Price, convey.ShouldEqual, 50000.1)
			convey.So(ticks[0].Volume, convey.ShouldEqual, 0.25)
			convey.So(ticks[0].Side, convey.ShouldEqual, "buy")
			convey.So(ticks[1].Side, convey.ShouldEqual, "sell")
		})
	})
}

func TestParseTradesRejectsNonTradeChannel(t *testing.T) {
	convey.Convey("Given a non-trade websocket frame", t, func() {
		convey.Convey("It should reject the payload", func() {
			_, err := ParseTrades(sampleBookFrame)
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}

func TestChannelName(t *testing.T) {
	convey.Convey("Given a websocket frame with a channel field", t, func() {
		convey.Convey("It should read the channel without full unmarshaling", func() {
			channel, err := ChannelName(sampleTradeFrame)
			convey.So(err, convey.ShouldBeNil)
			convey.So(channel, convey.ShouldEqual, "trade")
		})
	})
}
