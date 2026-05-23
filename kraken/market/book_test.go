package market

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestParseTopBook(t *testing.T) {
	convey.Convey("Given a Kraken v2 book websocket frame", t, func() {
		convey.Convey("It should extract the top bid and ask", func() {
			top, err := ParseTopBook(sampleBookFrame)
			convey.So(err, convey.ShouldBeNil)
			convey.So(top.BestBid.Price, convey.ShouldEqual, 49999.5)
			convey.So(top.BestBid.Volume, convey.ShouldEqual, 1.2)
			convey.So(top.BestAsk.Price, convey.ShouldEqual, 50000.5)
			convey.So(top.BestAsk.Volume, convey.ShouldEqual, 0.8)
		})
	})
}

func TestParseTopBookRejectsNonBookChannel(t *testing.T) {
	convey.Convey("Given a non-book websocket frame", t, func() {
		convey.Convey("It should reject the payload", func() {
			_, err := ParseTopBook(sampleTradeFrame)
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}
